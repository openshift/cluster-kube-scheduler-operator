package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	operatorsv1alpha1api "github.com/openshift/api/operator/v1alpha1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KubeSchedulerConfig provides information to configure kube-scheduler
type KubeSchedulerConfig struct {
	metav1.TypeMeta `json:",inline"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KubeSchedulerOperatorConfig provides information to configure an operator to manage kube-scheduler.
type KubeSchedulerOperatorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata" protobuf:"bytes,1,opt,name=metadata"`

	Spec   KubeSchedulerOperatorConfigSpec   `json:"spec" protobuf:"bytes,2,opt,name=spec"`
	Status KubeSchedulerOperatorConfigStatus `json:"status" protobuf:"bytes,3,opt,name=status"`
}

type KubeSchedulerOperatorConfigSpec struct {
	operatorsv1alpha1api.OperatorSpec `json:",inline" protobuf:"bytes,1,opt,name=operatorSpec"`

	// kubeSchedulerConfig holds a sparse config that the user wants for this component.  It only needs to be the overrides from the defaults
	// it will end up overlaying in the following order:
	// 1. hardcoded default
	// 2. this config
	KubeSchedulerConfig runtime.RawExtension `json:"kubeSchedulerConfig" protobuf:"bytes,2,opt,name=kubeSchedulerConfig"`
}

type KubeSchedulerOperatorConfigStatus struct {
	operatorsv1alpha1api.OperatorStatus `json:",inline" protobuf:"bytes,1,opt,name=operatorStatus"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KubeSchedulerOperatorConfigList is a collection of items
type KubeSchedulerOperatorConfigList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	// Items contains the items
	Items []KubeSchedulerOperatorConfig `json:"items" protobuf:"bytes,2,rep,name=items"`
}
