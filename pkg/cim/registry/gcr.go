package registry

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type GcrRegistry struct {
	cache       *cache
	credentials authn.Keychain
	baseURL     string
	log         logr.Logger
}

func (g *GcrRegistry) ResolveImages(ctx context.Context, directory string) ([]string, error) {
	ctx, cnFnc := context.WithTimeout(ctx, ResovleImageDefaultTimeout)
	defer cnFnc()
	log := g.log.WithValues("Directory", directory)

	ref, err := name.ParseReference(path.Join(g.baseURL, directory))
	if err != nil {
		return nil, fmt.Errorf("Invalid registry directory/repo ref: %s (Error Detail: %w)", path.Join(g.baseURL, directory), err)
	}

	log.V(1).Info("Resolving images")

	// Cache
	if images, ok := g.cache.GetImages(directory); ok {
		log.V(2).Info("Using value from cache", "Images", images)
		return images, nil
	}

	catalog, err := remote.Catalog(ctx, ref.Context().Registry, remote.WithAuthFromKeychain(g.Keychain()))
	if err != nil {
		return nil, fmt.Errorf("Error enumerating repositories: %w", err)
	}

	if len(catalog) == 0 {
		log.Info("Empty image repository")
		return nil, nil
	}

	// GCR returns images without base domain
	// + annoyingly always returns all repos that can be accessed using current credentials
	// -> filter out what we didn't want
	var images []string
	for i := range catalog {
		if strings.Contains(catalog[i], "/"+directory+"/") {
			images = append(images, path.Join(g.baseURL, catalog[i]))
		}
	}

	g.cache.SetImages(directory, images)
	return images, nil
}

func (g *GcrRegistry) ResolveTags(ctx context.Context, image string) ([]*TagDetails, error) {
	ctx, cnFnc := context.WithTimeout(ctx, ResolveTagsDefaultTimeout)
	defer cnFnc()
	log := g.log.WithValues("Image", image)

	// TODO: Enforce timout
	_ = ctx

	log.V(1).Info("Resolving tags")
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, fmt.Errorf("Invalid image ref: %s (Error Detail: %w)", image, err)
	}

	// Cache
	if tags, ok := g.cache.GetTags(image); ok {
		log.V(2).Info("Using value from cache", "Tags", tags)
		return tags, nil
	}

	gtags, err := google.List(ref.Context(), google.WithAuthFromKeychain(g.Keychain()))
	if err != nil {
		return nil, fmt.Errorf("Error listing tags: %w", err)
	}

	var tags []*TagDetails
	for digest, manifest := range gtags.Manifests {
		timestamp := manifest.Uploaded
		// GCR organizes stuff a bit more efficiently than others:
		// Indirection via digest, since multiple tags can point to the same image data
		// We (for compatibility with our 3rd party deps) process tag-centric instead of digest-centric
		// -> multiply the received digest to multiple tags for further processing in CIM
		for _, gtag := range manifest.Tags {
			tags = append(tags, &TagDetails{
				Original:     fmt.Sprintf("%s:%s", image, gtag),
				Image:        image,
				Digest:       digest,
				LastModified: &timestamp,
			})
		}
	}

	g.cache.SetTags(image, tags)
	return tags, nil
}

func (g *GcrRegistry) ResolveTagDetails(ctx context.Context, tag *TagDetails) (*TagDetails, error) {
	// Maybe there is nothing to do, since ResolveTags already populates the digest
	if tag.Digest != "" && tag.Size != 0 {
		return tag, nil
	}

	ctx, cnFnc := context.WithTimeout(ctx, ResolveTagsDefaultTimeout)
	defer cnFnc()

	// TODO: Enforce timeout
	_ = ctx

	ref, err := name.ParseReference(tag.String())
	if err != nil {
		return nil, fmt.Errorf("Invalid ref: %s (Error Detail: %w)", tag, err)
	}
	log := g.log.WithValues("Image", ref.Context().RepositoryStr(), "Tag", ref.Identifier())
	log.V(1).Info("Resolving tag details")

	// Cache
	if td, ok := g.cache.GetTagDetails(tag.String()); ok {
		log.V(2).Info("Using value from cache", "TagDetails", td)
		return td, nil
	}

	// Retrieve tag details from registry
	img, err := remote.Image(ref, remote.WithAuthFromKeychain(g.Keychain()))
	if err != nil {
		return nil, fmt.Errorf("Error listing tag details: %w", err)
	}
	digest, err := img.Digest()
	if err != nil {
		return nil, fmt.Errorf("Error retrieving tag digest: %w", err)
	}
	size, err := img.Size()
	if err != nil {
		return nil, fmt.Errorf("Error retrieving tag size: %w", err)
	}

	tag.Digest = digest.String()
	tag.Size = uint64(size)

	g.cache.SetTagDetails(tag.String(), tag)
	return tag, nil
}

func (g *GcrRegistry) Keychain() authn.Keychain {
	if g.credentials == nil {
		g.credentials = google.Keychain
	}
	return g.credentials
}

func (g *GcrRegistry) StripBaseURL(ref string) (string, error) {
	parts := strings.Split(ref, "/")
	if len(parts) < 3 {
		return "", fmt.Errorf("Invalid image/repo/directory ref: %s", ref)
	}
	return strings.Join(parts[2:], "/"), nil
}

func (g *GcrRegistry) String() string {
	return g.baseURL
}
