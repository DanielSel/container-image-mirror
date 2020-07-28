package registry

import (
	"time"
)

type TagDetails struct {
	// Image name including registry url and repo, e.g. "docker.io/danielsel/test"
	Image string
	// Digest is the image tag digest
	Digest string
	// Original remembers the original tag url used for the details request
	Original string
	// LastModified stores the last modified timestamp
	LastModified *time.Time
	// Size in bytes
	Size uint64
}

func (t *TagDetails) Equals(b *TagDetails) bool { return t.Digest == b.Digest }

func (t *TagDetails) String() string { return t.Original }
