package registry

import (
	"context"
	"testing"

	"github.com/danielsel/container-image-mirror/pkg/testlogr"
)

func Test_DockerHub_ResolveImages(t *testing.T) {
	// Testdata
	directoryPath := "danielsel"
	directoryURL := "docker.io/" + directoryPath

	// Arrange
	log := testlogr.NewTestLogger(t)
	registry, err := FromImageRef(log, directoryURL)
	if err != nil {
		t.Fatalf("Error instantiating registry client: %s", err)
	}

	// Act
	images, err := registry.ResolveImages(context.Background(), directoryPath)
	if err != nil {
		t.Fatalf("Error resolving images: %s", err)
	}
	log.V(1).Info("Retrieved images: %#v")

	// Assert
	if len(images) < 2 {
		t.Fatalf("Expected at least 2 images, got %d", len(images))
	}

}
