/*
Copyright 2020 Daniel Sel.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MirrorConfigSpec defines the desired state of MirrorConfig
type MirrorConfigSpec struct {
	Images       []string   `json:"images"`
	Destinations []string   `json:"destinations"`
	TagPolicy    *TagPolicy `json:"tagPolicy,omitempty"`
}

// MirrorConfigStatus defines the observed state of MirrorConfig
type MirrorConfigStatus struct {
	LastExecution *metav1.Time `json:"lastExecution,omitempty"`
	Summary       []string     `json:"summary,omitempty"`
}

// +kubebuilder:object:root=true

// MirrorConfig is the Schema for the mirrorconfigs API
type MirrorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MirrorConfigSpec   `json:"spec,omitempty"`
	Status MirrorConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MirrorConfigList contains a list of MirrorConfig
type MirrorConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MirrorConfig `json:"items"`
}

type TagPolicy struct {
	MinNum int             `json:"minNum,omitempty"`
	MaxNum int             `json:"maxNum,omitempty"`
	MaxAge metav1.Duration `json:"maxAge,omitempty"`
}

func init() {
	SchemeBuilder.Register(&MirrorConfig{}, &MirrorConfigList{})
}
