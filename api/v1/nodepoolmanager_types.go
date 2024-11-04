/*
Copyright 2024.

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

package v1

import (
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Action string

const (
	Create = "create"
	Update = "update"
	Delete = "delete"
)

type NodePool struct {
	Name   string                                                       `json:"name,omitempty"`
	Action Action                                                       `json:"action,omitempty"`
	Props  armcontainerservice.ManagedClusterAgentPoolProfileProperties `json:"properties,omitempty"`
}

// NodePoolManagerSpec defines the desired state of NodePoolManager
type NodePoolManagerSpec struct {
	SubscriptionID string     `json:"subscriptionId,omitempty"`
	ResourceGroup  string     `json:"resourceGroup,omitempty"`
	ClusterName    string     `json:"clusterName,omitempty"`
	RetryOnError   bool       `json:"retryOnError,omitempty"`
	TestMode       bool       `json:"testMode,omitempty"`
	NodePools      []NodePool `json:"nodePools"`
}

// NodePoolManagerStatus defines the observed state of NodePoolManager
type NodePoolManagerStatus struct {
	ClusterVersion string     `json:"clusterVersion,omitempty"`
	NodePools      []NodePool `json:"nodePools"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NodePoolManager is the Schema for the nodepoolmanagers API
type NodePoolManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodePoolManagerSpec   `json:"spec,omitempty"`
	Status NodePoolManagerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodePoolManagerList contains a list of NodePoolManager
type NodePoolManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodePoolManager `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodePoolManager{}, &NodePoolManagerList{})
}
