/*
Copyright 2022 The Crossplane Authors.

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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// KeyParameters are the configurable fields of a Key.
type KeyParameters struct {
	Duration       string            `json:"duration,omitempty"`
	KeyAlias       string            `json:"key_alias,omitempty"`
	Key            string            `json:"key,omitempty"`
	TeamID         string            `json:"team_id,omitempty"`
	UserID         string            `json:"user_id,omitempty"`
	Models         []string          `json:"models,omitempty"`
	MaxBudget      float64           `json:"max_budget,omitempty"`
	BudgetDuration string            `json:"budget_duration,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// KeyObservation are the observable fields of a Key.
type KeyObservation struct {
	Key     string      `json:"key,omitempty"`
	Expires metav1.Time `json:"expires,omitempty"`
	UserID  string      `json:"user_id,omitempty"`
	Status  string      `json:"status,omitempty"` // e.g., "generated"
}

// A KeySpec defines the desired state of a Key.
type KeySpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       KeyParameters `json:"forProvider"`
}

// A KeyStatus represents the observed state of a Key.
type KeyStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          KeyObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A Key is an example API type.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,litellm}
type Key struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeySpec   `json:"spec"`
	Status KeyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KeyList contains a list of Key
type KeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Key `json:"items"`
}

// Key type metadata.
var (
	KeyKind             = reflect.TypeOf(Key{}).Name()
	KeyGroupKind        = schema.GroupKind{Group: Group, Kind: KeyKind}.String()
	KeyKindAPIVersion   = KeyKind + "." + SchemeGroupVersion.String()
	KeyGroupVersionKind = SchemeGroupVersion.WithKind(KeyKind)
)

func init() {
	SchemeBuilder.Register(&Key{}, &KeyList{})
}
