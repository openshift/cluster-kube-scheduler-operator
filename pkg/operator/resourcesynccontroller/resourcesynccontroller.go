package resourcesynccontroller

import (
	"k8s.io/client-go/kubernetes"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
)

func NewResourceSyncController(
	operatorConfigClient v1helpers.OperatorClient,
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces,
	kubeClient kubernetes.Interface,
	eventRecorder events.Recorder) (*resourcesynccontroller.ResourceSyncController, error) {

	resourceSyncController := resourcesynccontroller.NewResourceSyncController(
		operatorConfigClient,
		kubeInformersForNamespaces,
		kubeClient,
		eventRecorder,
	)

	return resourceSyncController, nil
}
