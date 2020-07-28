package registry

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
)

var (
	regcache = make(map[string]Registry)
)

type Registry interface {
	fmt.Stringer
	ResolveImages(ctx context.Context, directory string) ([]string, error)
	ResolveTags(ctx context.Context, image string) ([]*TagDetails, error)
	ResolveTagDetails(ctx context.Context, tag *TagDetails) (*TagDetails, error)
	Keychain() authn.Keychain
	StripBaseURL(ref string) (string, error)
}

func FromImageRef(log logr.Logger, image string) (Registry, error) {
	parts := strings.Split(image, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("Invalid image ref: %s", image)
	}

	registryURL := parts[0]
	switch {
	case strings.HasSuffix(registryURL, "docker.io"):
		registry, ok := regcache[registryURL]
		if !ok {
			log := log.WithValues("Registry", "DockerHub")
			registry = &DockerHubRegistry{cache: newCache(log), log: log, baseURL: registryURL}
			regcache[registryURL] = registry
		}
		return registry, nil
	case strings.HasSuffix(registryURL, "gcr.io"):
		repoBaseUrl := path.Join(parts[0:1]...)
		registry, ok := regcache[repoBaseUrl]
		if !ok {
			log := log.WithValues("Registry", "GCR")
			registry = &GcrRegistry{cache: newCache(log), log: log, baseURL: registryURL}
			regcache[repoBaseUrl] = registry
		}
		return registry, nil
	}

	return nil, fmt.Errorf("Unsupported registry with URL: %s", registryURL)
}
