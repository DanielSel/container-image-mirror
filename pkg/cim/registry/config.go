package registry

import "time"

var (
	CacheDefaultTTL             = 15 * time.Minute
	CacheDefaultCleanupInterval = 5 * time.Minute
	ResovleImageDefaultTimeout  = 3 * time.Minute
	ResolveTagsDefaultTimeout   = 1 * time.Minute
)
