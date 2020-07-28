package cim

import (
	"fmt"
	"io/ioutil"
	"runtime"
	"time"

	cimapi "github.com/danielsel/container-image-mirror/api"
	"sigs.k8s.io/yaml"
)

var (
	ConfigGlobalTagLimit          uint          = 1024
	ConfigResolveImagesTimeout    time.Duration = 3 * time.Minute
	ConfigResolveTagsTimeout      time.Duration = 3 * time.Minute
	ConfigCopyImageDefaultTimeout time.Duration = 60 * time.Minute
	ConfigCopyImageTimeout        time.Duration = ConfigCopyImageDefaultTimeout
	ConfigDeleteImageTimeout      time.Duration = 5 * time.Minute
	ConfigEventProcessingTimeout  time.Duration = 10 * time.Second
	ConfigBufferSize              uint          = ConfigGlobalTagLimit
	ConfigNumWorkers              uint          = 8 * uint(runtime.NumCPU())
	ConfigNumParallelCopyJobs     uint          = 4 * uint(runtime.NumCPU())
	ConfigMaxMemBytes             uint64        = 6 * 1024 * 1024 * 1024
)

func MirrorConfigFromFile(path string) (*cimapi.MirrorConfig, error) {
	cfgdata, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Error reading config file: %w", err)
	}
	cfg := &cimapi.MirrorConfig{}
	if err := yaml.Unmarshal(cfgdata, cfg); err != nil {
		return nil, fmt.Errorf("Error parsing config file: %w", err)
	}
	return cfg, nil
}
