package registry

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

type DockerHubRegistry struct {
	cache       *cache
	credentials authn.Keychain
	baseURL     string
	log         logr.Logger
}

func (d *DockerHubRegistry) ResolveImages(ctx context.Context, directory string) ([]string, error) {
	ctx, cnFnc := context.WithTimeout(ctx, ResovleImageDefaultTimeout)
	defer cnFnc()
	log := d.log.WithValues("Directory", directory)

	ref, err := name.ParseReference(path.Join(d.baseURL, directory))
	if err != nil {
		return nil, fmt.Errorf("Invalid registry directory/repo ref: %s (Error Detail: %w)", path.Join(d.baseURL, directory), err)
	}

	// Cache
	if images, ok := d.cache.GetImages(directory); ok {
		log.V(2).Info("Using value from cache", "Images", images)
		return images, nil
	}

	client, err := NewRegistryClient(ref.Context().Registry, d.Keychain(), ref.Scope(transport.PullScope))
	if err != nil {
		return nil, err
	}

	uri := &url.URL{
		Scheme:   ref.Context().Registry.Scheme(),
		Host:     "hub.docker.com",
		Path:     fmt.Sprintf("/v2/repositories/%s", directory),
		RawQuery: "n=10000&page_size=10000",
	}

	parsed := &dockerHubRepoResults{}
	if err := ExecuteRequest(ctx, client, "GET", uri.String(), parsed); err != nil {
		return nil, err
	}

	if parsed.Results == nil {
		return nil, fmt.Errorf("Empty results table in json response: %w", err)
	}

	var images []string
	for _, repo := range parsed.Results {
		if repo.RepositoryType != "image" {
			continue
		}
		newRef, err := name.ParseReference(path.Join(d.baseURL, repo.Namespace, repo.Name))
		if err != nil {
			log.Error(err, "Invalid reference",
				"baseURL", d.baseURL,
				"repo.Namespace", repo.Namespace,
				"repo.Name", repo.Name,
				"repo.User", repo.User,
			)
		}

		images = append(images, newRef.String())
	}

	d.cache.SetImages(directory, images)
	return images, nil
}

func (d *DockerHubRegistry) ResolveTags(ctx context.Context, image string) ([]*TagDetails, error) {
	ctx, cnFnc := context.WithTimeout(ctx, ResolveTagsDefaultTimeout)
	defer cnFnc()
	log := d.log.WithValues("Image", image)

	log.V(1).Info("Resolving tags")
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, fmt.Errorf("Invalid image ref: %s (Error Detail: %w)", image, err)
	}

	// Cache
	if tags, ok := d.cache.GetTags(image); ok {
		log.V(2).Info("Using value from cache", "Tags", tags)
		return tags, nil
	}

	client, err := NewRegistryClient(ref.Context().Registry, d.Keychain(), ref.Scope(transport.PullScope))
	if err != nil {
		return nil, err
	}

	uri := &url.URL{
		Scheme:   ref.Context().Registry.Scheme(),
		Host:     "hub.docker.com",
		Path:     fmt.Sprintf("/v2/repositories/%s/tags", ref.Context().RepositoryStr()),
		RawQuery: "n=10000&page_size=10000",
	}

	parsed := &dockerHubTagResults{}
	if err := ExecuteRequest(ctx, client, "GET", uri.String(), parsed); err != nil {
		return nil, err
	}

	if parsed.Results == nil {
		return nil, fmt.Errorf("Empty results table in json response: %w", err)
	}

	var tags []*TagDetails
	for _, dhtag := range parsed.Results {
		timestamp := dhtag.LastUpdated
		if dhtag.Images == nil {
			tags = append(tags, &TagDetails{
				Original:     fmt.Sprintf("%s:%s", image, dhtag.Name),
				Image:        image,
				LastModified: &timestamp,
				Size:         dhtag.FullSize,
			})
			continue
		}
		// If digest(s) are available, add to tag(s)
		for _, dhImgRef := range dhtag.Images {
			tags = append(tags, &TagDetails{
				Original:     fmt.Sprintf("%s:%s", image, dhtag.Name),
				Image:        image,
				Digest:       dhImgRef.Digest,
				LastModified: &timestamp,
				Size:         dhImgRef.Size,
			})
		}
	}

	d.cache.SetTags(image, tags)
	return tags, nil
}

func (d *DockerHubRegistry) ResolveTagDetails(ctx context.Context, tag *TagDetails) (*TagDetails, error) {
	// Maybe there is nothing to do, since ResolveTags already populates the digest
	if tag.Digest != "" && tag.Size != 0 {
		return tag, nil
	}

	ctx, cnFnc := context.WithTimeout(ctx, ResolveTagsDefaultTimeout)
	defer cnFnc()
	// TODO: Enforce timeout
	_ = ctx

	if tag == nil {
		return nil, errors.New("Tag is nil")
	}

	ref, err := name.ParseReference(tag.String())
	if err != nil {
		return nil, fmt.Errorf("Invalid ref: %s (Error Detail: %w)", tag, err)
	}
	log := d.log.WithValues("Image", ref.Context().RepositoryStr(), "Tag", ref.Identifier())
	log.V(1).Info("Resolving tag details")

	// Cache
	if td, ok := d.cache.GetTagDetails(tag.String()); ok {
		log.V(2).Info("Using value from cache", "TagDetails", td)
		return td, nil
	}

	// Retrieve tag details from registry
	img, err := remote.Image(ref, remote.WithAuthFromKeychain(d.Keychain()))
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

	d.cache.SetTagDetails(tag.String(), tag)
	return tag, nil
}

func (d *DockerHubRegistry) Keychain() authn.Keychain {
	if d.credentials == nil {
		d.credentials = authn.DefaultKeychain
	}
	return d.credentials
}

func (d *DockerHubRegistry) StripBaseURL(ref string) (string, error) {
	parts := strings.Split(ref, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("Invalid image/repo/directory ref: %s", ref)
	}
	return strings.Join(parts[1:], "/"), nil
}

func (d *DockerHubRegistry) String() string {
	return d.baseURL
}

type dockerHubRepoResults struct {
	Results []dockerHubRepo `json:"results"`
}

type dockerHubRepo struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	User           string `json:"user,omitempty"`
	RepositoryType string `json:"repository_type,omitempty"`
}

type dockerHubTagResults struct {
	Results []dockerHubTag `json:"results"`
}

type dockerHubTag struct {
	Name        string              `json:"name"`
	Images      []dockerHubImageRef `json:"images"`
	LastUpdated time.Time           `json:"last_updated"`
	FullSize    uint64              `json:"full_size"`
}

type dockerHubImageRef struct {
	Digest string `json:"digest"`
	Size   uint64 `json:"size"`
}
