package cim

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/crane"
	glogs "github.com/google/go-containerregistry/pkg/logs"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/rs/xid"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cimapi "github.com/danielsel/container-image-mirror/api"
	"github.com/danielsel/container-image-mirror/pkg/cim/registry"
	"github.com/danielsel/container-image-mirror/pkg/memlim"
)

func Run(config *cimapi.MirrorConfig, ctx context.Context, log logr.Logger) error {
	runner := NewRunner(config, log)
	return runner.Run(ctx)
}

func NewRunner(config *cimapi.MirrorConfig, log logr.Logger) *Runner {
	if log == nil {
		log = logf.NullLogger{}
	}
	runner := &Runner{
		config:        config,
		log:           log,
		status:        newRunnerStatus(),
		eventHandlers: []EventHandler{},
	}
	runner.eventHandlers = append(runner.eventHandlers, runner.updateStatus)
	return runner
}

type Runner struct {
	config        *cimapi.MirrorConfig
	log           logr.Logger
	status        *runnerStatus
	eventHandlers []EventHandler
}

type copyJob struct {
	src, dst *registry.TagDetails
	size     uint64
}

func (r *Runner) Run(ctx context.Context) error {
	// Configure logging and context
	log := r.log.WithName("CIM").WithValues("MirrorConfig", r.config.Name)

	// Configure logging for go-containerregistry package
	glogr := log.WithValues("Module", "go-containerregistry")
	glogs.Debug = WrapStdLogger(glogr.V(2))
	glogs.Warn = WrapStdLogger(glogr)
	glogs.Progress = WrapStdLogger(glogr.V(1))

	// Configure logging for memlim package
	memlim.SetLogger(log.WithName("System").WithValues("Module", "memlim"))

	// Configure memory limit
	memlim.LimitMemory(ConfigMaxMemBytes)
	// Let memlim know that we will provide hints about freed memory from our operations
	// to improve efficiency of our memory management
	memlim.FreedMemHint(0)

	// Start
	log.Info("Running mirror job", "MirrorConfig", *r.config)

	// Preflight Checks
	if err := r.preflightChecks(); err != nil {
		return fmt.Errorf("preflight checks failed: %w", err)
	}

	// Set up worker pool, processing pipelines, and execute
	// Prepare: Emits source directory strings through a channel
	prepared := r.prepare(ctx, r.config.Spec.Images)
	// Resolve each source directory into a list of image tags [worker pool]
	resolved, resolveEvtCh := r.resolve(ctx, prepared)
	// Filter image tags based on policy
	filtered := r.filter(ctx, resolved)
	// Transform each image tag FQDN into multiple copyJobs (one per mirror destination)
	transformed := r.transform(ctx, r.config.Spec.Destinations, filtered)
	td1, td2 := splitJobCh(transformed)
	// Interleave copy jobs to avoid (if possible) copying multiple tags of the same image in parallel (optimization)
	interleaved := r.interleave(ctx, td1)
	// Run crane.copy for each copyJob [worker pool]
	copyEvtCh := r.copy(ctx, r.config.Spec.Destinations, interleaved)

	// Housekeeping (in flow after 'filter' stage and parallel to 'copy' stage)
	hkEvtCh := r.housekeeping(ctx, td2)

	// Aggregate and process events
	evtCh := mergeEventChs(resolveEvtCh, copyEvtCh, hkEvtCh)
	for evt := range evtCh {
		ctx, cnFnc := context.WithTimeout(ctx, ConfigEventProcessingTimeout)
		r.processEvent(ctx, evt)
		cnFnc()
	}

	// Notify user to check error log
	if failures := r.status.Count("", CounterTypeFailed); failures > 0 {
		err := errors.New("container-image-mirror execution failed. Please check the error log.")
		log.Info("container-image-mirror execution failed. At least some images could not be copied successfully. Please check the error log.",
			"failures", failures,
		)
		return err
	}

	return nil
}

func (r *Runner) Status() runnerStatus {
	return *r.status
}

func (r *Runner) prepare(ctx context.Context, sources []string) chan string {
	log := r.log.WithValues("Stage", "Prepare")
	out := make(chan string)
	go func() {
		for _, source := range sources {
			out <- source
			log.V(1).Info("Emitting source directory", "source", source)
		}
		close(out)
	}()
	return out
}

func (r *Runner) resolve(ctx context.Context, in chan string) (chan *registry.TagDetails, chan Event) {
	log := r.log.WithValues("Stage", "Resolve")
	log.Info("Resolving images from source directories")
	log.V(1).Info("Creating worker pool for RESOLVE stage", "WorkerPoolSize", ConfigNumWorkers)
	var outs []chan *registry.TagDetails
	var evts []chan Event
	for i := 0; i < int(ConfigNumWorkers); i++ {
		// This is where the action happens. Rest of the function is boilerplate
		// Wire resolve workers to resolve SourceDirs to Images, Images to Tags, and Tags to TagDetails
		// + Aggregate event channels
		images, evt1 := r.resolveImages(ctx, in)
		tags, evt2 := r.resolveTags(ctx, images)
		// TODO: Determine if more parallelization here increases performance
		tagdetails, evt3 := r.resolveTagDetails(ctx, tags)
		outs = append(outs, tagdetails)
		evts = append(evts, evt1, evt2, evt3)
	}

	return mergeTagDetailChs(outs...), mergeEventChs(evts...)
}

// filter applies tag policies and detects changes between filtered source and destination tags
func (r *Runner) filter(ctx context.Context, in chan *registry.TagDetails) chan *registry.TagDetails {
	log := r.log.WithValues("Stage", "Filter")
	log.Info("Filtering images/tags")
	log.V(1).Info("Creating worker pool for FILTER stage", "WorkerPoolSize", ConfigNumWorkers)
	out := make(chan *registry.TagDetails)

	// Cache Tags in a list
	go func() {
		tagmap := make(map[string][]*registry.TagDetails)
		for x := range in {
			tagmap[x.Image] = append(tagmap[x.Image], x)
		}

		// Treat tag collections separately per image
		// TODO: parallel (go routines)
		for _, tags := range tagmap {
			// Sort Tags by age (youngest first)
			sort.Slice(tags, func(i, j int) bool {
				iTag := tags[i]
				jTag := tags[j]
				if iTag.LastModified == nil || jTag.LastModified == nil {
					return false
				}
				return iTag.LastModified.After(*jTag.LastModified)
			})

			// Filtered Tags
			ftags := make([]*registry.TagDetails, 0, r.config.Spec.TagPolicy.MinNum)

			// Tag Policy: Copy tags fulfilling 'maxAge' requirement
			thresholdTime := time.Now().Add(-1 * r.config.Spec.TagPolicy.MaxAge.Duration)
			blacklist := make(map[string]struct{})
			for _, x := range tags {
				if x.LastModified.After(thresholdTime) {
					ftags = append(ftags, x)
					blacklist[x.String()] = struct{}{}
				}
			}

			// Tag Policy: Ensure 'minNum' requirement
			ix := 0
			for len(ftags) < r.config.Spec.TagPolicy.MinNum && len(tags) > len(ftags) {
				tag := tags[ix]
				if _, ok := blacklist[tag.String()]; !ok {
					// Only copy if not copied in previous step (blacklisted)
					ftags = append(ftags, tag)
				}
				ix++
			}

			// Tag Policy: Ensure 'maxNum' requirement
			if len(ftags) > r.config.Spec.TagPolicy.MaxNum {
				ftags = ftags[:r.config.Spec.TagPolicy.MaxNum]
			}

			// Send filtered tags to output channels
			for _, ftag := range ftags {
				out <- ftag
			}
		}

		// Send filtered tag strings to output channels
		// for _, tags := range tagmap {
		// 	for _, td := range tags {
		// 		fmt.Println(fmt.Sprintf("[Time: %s] %s", td.LastModified, td.String()))
		// 		// out <- td.String()
		// 	}
		// }
		close(out)
	}()

	return out
}

// transform multiplies every image by the number of associated target mirrors
// and transforms the image name to match the destination registry.
func (r *Runner) transform(ctx context.Context, mirrorRepos []string, in chan *registry.TagDetails) chan copyJob {
	log := r.log.WithValues("Stage", "Transform")
	out := make(chan copyJob)
	go func() {
		for x := range in {
			for _, mirror := range mirrorRepos {
				dst := &registry.TagDetails{Original: path.Join(mirror, x.String())}
				out <- copyJob{src: x, dst: dst, size: x.Size}
				log.V(1).Info("Transform: Add Copy Job",
					"src", x.String(), "dst", dst)

			}
		}
		close(out)
	}()
	return out
}

// interleave transforms the active stream by reordering subsets of copyJobs.
// The goal is to avoid (if possible) two tags of the same image being copied in parallel
// since locally restricted sequential copy would allow to use remote registry mounts
// for shared layers between tags of the same image
func (r *Runner) interleave(ctx context.Context, in chan copyJob) chan copyJob {
	// Create buffer for interleaved jobs
	buf := NewInterleaveBuffer(ConfigNumParallelCopyJobs)
	out := make(chan copyJob)
	go func() {
		for x := range in {
			inJob := x
			outJob := buf.Tick(&inJob)
			if outJob != nil {
				out <- *outJob
			}
		}
		buf.FlushBuffer(out)
		close(out)
	}()
	return out
}

// copy executes copyJobs in a worker pool
func (r *Runner) copy(ctx context.Context, mirrorRepos []string, in chan copyJob) chan Event {
	log := r.log.WithValues("Stage", "Copy")
	log.Info("Copying images to mirror repositories using crane.")
	log.V(1).Info("Creating worker pool for COPY stage", "WorkerPoolSize", ConfigNumParallelCopyJobs)
	var evts []chan Event
	for i := 0; i < int(ConfigNumParallelCopyJobs); i++ {
		// This is where the action happens. Rest of the function is boilerplate
		evts = append(evts, r.copyImage(ctx, in))
	}

	return mergeEventChs(evts...)
}

// housekeeping ensures that only the desired (specified) image tags are present
// in the destination (mirror) paths
func (r *Runner) housekeeping(ctx context.Context, in chan copyJob) chan Event {
	log := r.log.WithValues("Stage", "Housekeeping")
	log.Info("Cleaning up undesired images from mirror registry.")

	log.V(1).Info("Creating worker pool", "WorkerPoolSize", ConfigNumWorkers)
	deltags := make(chan string)
	var evts []chan Event
	for i := 0; i < int(ConfigNumWorkers); i++ {
		// Run tag deletion using worker pool
		evts = append(evts, r.deleteTag(ctx, deltags))
	}

	// Setup pre-processing in the background
	// 'deltags' channel is where the delete orders from processing will arrive
	// Prepare: Emits destination directory strings through a channel
	prepared := r.prepare(ctx, r.config.Spec.Destinations)
	// Resolve each destination directory into a list of image tags [worker pool]
	resolved, resolveEvtCh := r.resolve(ctx, prepared)
	evts = append(evts, resolveEvtCh)
	go func() {
		// Track which tags should be deleted
		delTagsMap := make(map[string]struct{})

		// Load all images + tags from destination paths
		for td := range resolved {
			delTagsMap[td.String()] = struct{}{}
		}

		// Remove all tags that we want to keep
		for tag := range in {
			delete(delTagsMap, tag.dst.String())
		}

		// Delete the rest
		for x := range delTagsMap {
			deltags <- x
		}
		close(deltags)
	}()

	return mergeEventChs(evts...)
}

func (r *Runner) resolveImages(ctx context.Context, in chan string) (chan string, chan Event) {
	ctx, cnFnc := context.WithTimeout(ctx, ConfigResolveImagesTimeout)
	log := r.log.WithValues("Stage", "ResolveImages")
	out := make(chan string)
	evt := make(chan Event)
	go func() {
		for source := range in {
			log.V(1).Info("Resolving source directory", "source", source)
			reg, err := registry.FromImageRef(r.log, source)
			if err != nil {
				log.Error(err, "Error instantiating registry client",
					"Source", source,
				)
				emit(evt, NewStatusEvent(source, ObjectTypeSourceDir, StatusTypeResolved, false))
				continue
			}

			directory, err := reg.StripBaseURL(source)
			if err != nil {
				log.Error(err, "Error determining directory from image url",
					"Source", source,
				)
				emit(evt, NewStatusEvent(source, ObjectTypeSourceDir, StatusTypeResolved, false))
				continue
			}

			images, err := reg.ResolveImages(ctx, directory)
			if err != nil {
				log.Error(err, "Error resolving images",
					"Source", source,
				)
				emit(evt, NewStatusEvent(source, ObjectTypeSourceDir, StatusTypeResolved, false))
				continue
			}

			for _, img := range images {
				out <- img
				log.V(1).Info("Resolved image from source directory",
					"Source", source, "Image", img)
			}
			log.V(1).Info("Resolved all images from source directory",
				"Source", source)
			emit(evt, NewStatusEvent(source, ObjectTypeSourceDir, StatusTypeResolved, true))
		}
		close(out)
		close(evt)
		cnFnc()
	}()
	return out, evt
}

func (r *Runner) resolveTags(ctx context.Context, in chan string) (chan *registry.TagDetails, chan Event) {
	ctx, cnFnc := context.WithTimeout(ctx, ConfigCopyImageTimeout)
	log := r.log.WithValues("Stage", "ResolveTags")
	out := make(chan *registry.TagDetails)
	evt := make(chan Event)
	go func() {
		for x := range in {
			log.Info("Resolving image tags", "Image", x)
			reg, err := registry.FromImageRef(log, x)
			if err != nil {
				log.Error(err, "Error creating registry client for tag resolver",
					"Image", x,
				)
				emit(evt, NewStatusEvent(x, ObjectTypeImage, StatusTypeResolved, false))
				continue
			}
			tags, err := reg.ResolveTags(ctx, x)
			if err != nil {
				log.Error(err, "Error resolving tags",
					"Image", x)
				emit(evt, NewStatusEvent(x, ObjectTypeImage, StatusTypeResolved, false))
				continue
			}

			for _, tag := range tags {
				out <- tag
				log.V(1).Info("Resolved tag from image",
					"Image", x, "Tag", tag)
			}
			emit(evt, NewStatusEvent(x, ObjectTypeImage, StatusTypeResolved, true))
		}
		close(out)
		close(evt)
		cnFnc()
	}()
	return out, evt
}

func (r *Runner) resolveTagDetails(ctx context.Context, tags chan *registry.TagDetails) (chan *registry.TagDetails, chan Event) {
	ctx, cnFnc := context.WithTimeout(ctx, ConfigCopyImageTimeout)
	log := r.log.WithValues("Stage", "ResolveTagDetails")
	out := make(chan *registry.TagDetails)
	evt := make(chan Event)
	go func() {
		for tag := range tags {
			log.V(1).Info("Resolving tag details", "Tag", tag)
			reg, err := registry.FromImageRef(log, tag.String())
			if err != nil {
				log.Error(err, "Error creating registry client for tag resolver",
					"Tag", tag,
				)
				emit(evt, NewStatusEvent(tag.String(), ObjectTypeTag, StatusTypeResolved, false))
				continue
			}
			td, err := reg.ResolveTagDetails(ctx, tag)
			if err != nil {
				log.Error(err, "Error resolving tag details",
					"Tag", tag)
				emit(evt, NewStatusEvent(tag.String(), ObjectTypeTag, StatusTypeResolved, false))
				continue
			}

			out <- td
			emit(evt, NewStatusEvent(tag.String(), ObjectTypeTag, StatusTypeResolved, true))

		}
		close(out)
		close(evt)
		cnFnc()
	}()
	return out, evt
}

func (r *Runner) copyImage(ctx context.Context, in chan copyJob) chan Event {
	log := r.log.WithValues("Stage", "Copy")
	evt := make(chan Event)
	go func() {
		for x := range in {
			ctxTags, tagsCnFnc := context.WithTimeout(ctx, ConfigResolveTagsTimeout)
			log := log.WithValues("Src", x.src.String(), "Dst", x.dst.String())
			// Skip if the same
			var metaerr error
			reg, err := registry.FromImageRef(log, x.dst.String())
			metaerr = err
			if err == nil {
				targetTagDetail, err := reg.ResolveTagDetails(ctxTags, x.dst)
				metaerr = err
				if err == nil {
					if targetTagDetail.Equals(x.src) {
						log.Info("Image tag up to date, skipping copy operation")
						// TODO: New status for 'up-to-date'
						emit(evt, NewStatusEvent(x.src.String(), ObjectTypeTag, StatusTypeCopied, true))
						tagsCnFnc()
						continue
					}
				}
			}
			if metaerr != nil {
				err := &transport.Error{}
				if !errors.As(metaerr, &err) || err.StatusCode != http.StatusNotFound {
					log.Error(err, "Error resolving tag details for copy pre-flight checks")
				}
			}
			tagsCnFnc()

			// Copy
			ctxCopy, copyCnFnc := context.WithTimeout(ctx, ConfigCopyImageTimeout)
			memlim.WaitForFreeMemory(ctx, x.size,
				func() {
					// Cancel and requeue operation if we run out of memory
					copyCnFnc()
					log.Info("WARNING: cancelled copy operation due to memory pressure")
				})
			log.Info("Copying image tag")
			errch := make(chan error)
			go func() {
				// Make garbage collection great again
				// (faster GC for finished single go routines)
				errch <- crane.Copy(x.src.String(), x.dst.String(), func(opts *crane.Options) { opts.Remote = append(opts.Remote, remote.WithContext(ctxCopy)) })
				memlim.FreedMemHint(x.size)
			}()
			err = <-errch
			if err != nil {
				switch {
				case errors.Is(err, context.Canceled):
					log.Info("Context was cancelled, possibly due to memory pressure. Requeuing...")
					in <- x
					continue
				case errors.Is(err, context.DeadlineExceeded) && ConfigCopyImageTimeout == ConfigCopyImageDefaultTimeout:
					log.Info("Context deadline exceeded. Requeuing while temporarily doubling timeout...")
					ConfigCopyImageTimeout *= 2
					in <- x
					continue
				default:
					log.Error(err, "error copying image tag")
					emit(evt, NewStatusEvent(x.src.String(), ObjectTypeTag, StatusTypeCopied, false))
					copyCnFnc()
					continue
				}
			}
		}
		close(evt)
	}()
	return evt
}

func (r *Runner) deleteTag(ctx context.Context, in chan string) chan Event {
	ctx, cnFnc := context.WithTimeout(ctx, ConfigCopyImageTimeout)
	log := r.log.WithValues("Stage", "Housekeeping")
	evt := make(chan Event)
	go func() {
		for x := range in {
			log.Info("Deleting image tag", "Tag", x)
			if err := crane.Delete(x); err != nil {
				// TODO: Make crane.Delete respect ctx
				_ = ctx
				log.Error(err, "error deleting image tag", "Tag", x)
				emit(evt, NewStatusEvent(x, ObjectTypeTag, StatusTypeDeleted, false))
				continue
			}
			log.Info("Successfully deleted image tag", "Tag", x)
			emit(evt, NewStatusEvent(x, ObjectTypeTag, StatusTypeDeleted, true))
		}
		close(evt)
		cnFnc()
	}()
	return evt
}

func (r *Runner) eventSink(ctx context.Context, inputs ...chan Event) {
	log := r.log.WithValues("Stage", "EventProcessing")

	// Merge all event streams into single 'in' channel
	in := make(chan Event)
	var wg sync.WaitGroup
	forwarder := func(c <-chan Event) {
		for n := range c {
			in <- n
		}
		wg.Done()
	}
	wg.Add(len(inputs))
	for _, c := range inputs {
		go forwarder(c)
	}
	go func() {
		wg.Wait()
		close(in)
	}()

	// Process Events
	for evt := range in {
		r.processEvent(ctx, evt)
	}

	log.V(1).Info("Finished processing all events.")
}

func (r *Runner) processEvent(ctx context.Context, evt Event) {
	log := r.log.WithName("Event").WithValues("event", evt)
	log.V(1).Info("Processing event")
	for i, handler := range r.eventHandlers {
		log.V(2).Info(fmt.Sprintf("Invoking Event Handler #%d", i), "EventHandler", handler)
		err := handler(ctx, log, evt)
		if err != nil {
			log.Error(err, fmt.Sprintf("Error Handler #%d failed", i), "EventHandler", handler)
			continue
		}
	}
	log.V(1).Info("Finished processing event")
}

func (r *Runner) updateStatus(ctx context.Context, log logr.Logger, evt Event) error {
	switch evt.Type {
	case EventTypeStatusChange:
		data := evt.Data.(StatusEventData)
		status, err := r.status.StatusByObjType(data.ObjType)
		if err != nil {
			return fmt.Errorf("error retrieving runner status based on the event's object type: %w", err)
		}
		if err := status.Set(evt.Source.(string), data.StatusType, data.Success); err != nil {
			return fmt.Errorf("error setting status: %w", err)
		}
		log.V(2).Info("Status Updated",
			"Source", evt.Source.(string),
			"ObjectType", data.ObjType,
			"StatusType", data.StatusType,
			"Success", data.Success,
		)
	default:
		log.Info("Unknown event type", "event", evt)
	}
	return nil
}

func (r *Runner) preflightChecks() error {
	r.log.Info("Running preflight checks...")
	// Check Sources exist?
	if len(r.config.Spec.Images) == 0 {
		return errors.New("No source image directories configured. Aborting.")
	}

	// Check Destinations exist?
	if len(r.config.Spec.Destinations) == 0 {
		return errors.New("No destination registries configured. Aborting.")
	}

	// Check Destination Push Permissions
	var dstSuccess []string
	repo := path.Join("test", xid.New().String())
	for _, regurl := range r.config.Spec.Destinations {
		ref, err := name.ParseReference(path.Join(regurl, repo))
		if err != nil {
			return fmt.Errorf("Invalid preflight check repo '%s': %w", path.Join(regurl, repo), err)
		}
		reg, err := registry.FromImageRef(r.log, ref.String())
		if err != nil {
			return fmt.Errorf("Error instantiating registry client for repo '%s': %w", path.Join(regurl, repo), err)
		}

		if err := remote.CheckPushPermission(ref, reg.Keychain(), http.DefaultTransport); err != nil {
			r.log.Error(err, "Failed to verify push permission. Removing registry from destination set", "Registry", regurl)
			continue
		}

		dstSuccess = append(dstSuccess, regurl)
	}

	if len(dstSuccess) == 0 {
		return errors.New("None of the configured destination registries passed the push permission verification. Aborting.")
	}

	// Successfully filtered and verified destination registries
	r.config.Spec.Destinations = dstSuccess

	r.log.Info("Preflight checks successfully completed.")
	return nil
}
