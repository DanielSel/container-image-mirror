package api

import (
	cimapi "github.com/danielsel/container-image-mirror/api/v1alpha1"
)

var AddToScheme = cimapi.AddToScheme

type MirrorConfig = cimapi.MirrorConfig
type MirrorConfigList = cimapi.MirrorConfigList
type MirrorConfigSpec = cimapi.MirrorConfigSpec
type MirrorConfigStatus = cimapi.MirrorConfigStatus

type TagPolicy = cimapi.TagPolicy
