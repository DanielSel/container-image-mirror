package cim

import (
	"strings"
)

// NewInterleaveBuffer creates a new InterleaveBuffer that attempts to
// interleave multiple copyJobs while considering the <interval>
// between different tags of the same image
// Usually, interval should be set to the number of parallel go routines
// (aka size of the worker pool) in the COPY stage
func NewInterleaveBuffer(interval uint) *InterleaveBuffer {
	buf := make(chan *copyJob, ConfigBufferSize)
	// for i := 0; i < int(capacity); i++ {
	// 	buf <- nil
	// }
	return &InterleaveBuffer{
		buf:    buf,
		filter: make(map[string]uint),
		cap:    interval,
		epoch:  0,
	}
}

// InterleaveBuffer assists interleaving the copyJob stream
// with the goal of avoiding (if possible) two tags of the same image being copied in parallel
// since locally restricted sequential copy would allow to use remote registry mounts
// for shared layers between tags of the same image
type InterleaveBuffer struct {
	buf        chan *copyJob
	filter     map[string]uint
	cap, epoch uint
}

// Tick represents one interleaved iteration
// Every tick the buffer checks if the tag should be interleaved
// and returns the same tag or a replacement tag for the current epoch
func (i *InterleaveBuffer) Tick(in *copyJob) *copyJob {
	// Increase epoch every tick
	// TODO: Safe rollover at  max(uint)
	defer func() {
		i.epoch++
	}()

	// Tag -> Image reduction for interleaving
	image := stripTag(in.dst.String())
	imgEpoch, ok := i.filter[image]

	// Not in worker pool yet -> remember image and return exact same job
	if !ok {
		i.filter[image] = i.epoch + i.cap
		return in
	}

	// Already in worker pool -> check epoch
	// If expired (= processed by WP) -> delete from filter
	//  -> remember image and return exact same job (recursion)
	if imgEpoch <= i.epoch {
		i.filter[image] = i.epoch + i.cap
		return in
	}

	// Not expired -> matches? -> cache in buf
	i.buf <- in

	// Retrieve first one from channel that isn't in filter
	lim := len(i.buf)
	for x := range i.buf {
		if _, ok := i.filter[stripTag(x.dst.String())]; !ok {
			i.filter[stripTag(x.dst.String())] = i.epoch + i.cap
			return x
		}

		// Got a tag from buffer that belongs to an image currently in worker pool
		// -> Put back in the buffer
		i.buf <- x

		// Don't get stuck in infinite loop, stop after having cycled the whole buffer
		lim--
		if lim == 0 {
			break
		}
	}

	// Nil rounds shouldn't influence epoch
	i.epoch--
	return nil
}

// FlushBuffer handles ordered emptying of elements still in the buffer
// after all ticks are processed
func (i *InterleaveBuffer) FlushBuffer(out chan copyJob) {
	// Temporary and recursive flush buffer
	flushBuf := NewInterleaveBuffer(i.cap)
	close(i.buf)
	for x := range i.buf {
		if job := flushBuf.Tick(x); job != nil {
			out <- *job
		}
	}

	switch len(flushBuf.buf) {
	case 0:
		close(flushBuf.buf)
	case 1:
		out <- *<-flushBuf.buf
		close(flushBuf.buf)
	default:
		flushBuf.FlushBuffer(out)
	}
}

// Helper function to strip tag
func stripTag(tag string) string {
	parts := strings.Split(tag, ":")
	if len(parts) == 1 {
		return tag
	}
	image := parts[0]
	for i := 1; i < len(parts)-1; i++ {
		image += ":" + parts[i]
	}
	return image
}
