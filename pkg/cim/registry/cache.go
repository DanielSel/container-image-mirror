package registry

import (
	"reflect"

	"github.com/go-logr/logr"
	pcache "github.com/patrickmn/go-cache"
)

func newCache(log logr.Logger) *cache {
	return &cache{
		log:   log,
		cache: pcache.New(CacheDefaultTTL, CacheDefaultCleanupInterval),
	}
}

type cache struct {
	log   logr.Logger
	cache *pcache.Cache
}

func (c *cache) GetImages(directory string) ([]string, bool) {
	key := directory
	if val, ok := c.cache.Get(key); ok {
		if images, ok := val.([]string); ok {
			return images, true
		}
		c.log.WithName("System").Info("Unexpected value type from cache hit", "Key", key, "Value", val, "ValueType", reflect.TypeOf(val).String())
	}
	return nil, false
}

func (c *cache) SetImages(directory string, images []string) {
	c.cache.Set(directory, images, 0)
}

func (c *cache) GetTags(image string) ([]*TagDetails, bool) {
	key := image
	if val, ok := c.cache.Get(key); ok {
		if tags, ok := val.([]*TagDetails); ok {
			return tags, true
		}
		c.log.WithName("System").Info("Unexpected value type from cache hit", "Key", key, "Value", val, "ValueType", reflect.TypeOf(val).String())
	}
	return nil, false
}

func (c *cache) SetTags(image string, tags []*TagDetails) {
	c.cache.Set(image, tags, 0)
	// Save individual tags separately to improve cache accuracy downstream
	for _, tag := range tags {
		// Don't accidentally overwrite an existing tag with richer metadata
		if rawstag, ok := c.cache.Get(tag.String()); ok {
			if stag, ok := rawstag.(*TagDetails); ok {
				if stag.Digest != "" && tag.Digest == "" ||
					stag.LastModified != nil && tag.LastModified == nil {
					continue
				}
			}
		}

		c.cache.Set(tag.String(), tag, 0)
	}
}

func (c *cache) GetTagDetails(tag string) (*TagDetails, bool) {
	key := tag
	if val, ok := c.cache.Get(key); ok {
		if tag, ok := val.(*TagDetails); ok {
			// Ensure that tag details are populated and not just pre-caching from image resolution
			if tag.Digest == "" || tag.LastModified == nil {
				return nil, false
			}
			// Cache hit
			return tag, true
		}
		c.log.WithName("System").Info("Unexpected value type from cache hit", "Key", key, "Value", val, "ValueType", reflect.TypeOf(val).String())
	}
	return nil, false
}

func (c *cache) SetTagDetails(tag string, details *TagDetails) {
	c.cache.Set(tag, details, 0)
}
