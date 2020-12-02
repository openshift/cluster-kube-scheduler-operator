package resourcesynccontroller

import (
	"k8s.io/client-go/kubernetes"

	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
)

func AddSyncClientCertKeySecret(resourceSyncController *resourcesynccontroller.ResourceSyncController) error {
	return resourceSyncController.SyncSecret(
		resourcesynccontroller.ResourceLocation{
			Namespace: operatorclient.TargetNamespace,
			Name:      "kube-scheduler-client-cert-key",
		},
		resourcesynccontroller.ResourceLocation{
			Namespace: operatorclient.GlobalMachineSpecifiedConfigNamespace,
			Name:      "kube-scheduler-client-cert-key",
		},
	)
}

func NewResourceSyncController(
	operatorConfigClient v1helpers.OperatorClient,
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces,
	kubeClient kubernetes.Interface,
	eventRecorder events.Recorder) (*resourcesynccontroller.ResourceSyncController, error) {

	resourceSyncController := resourcesynccontroller.NewResourceSyncController(
		operatorConfigClient,
		kubeInformersForNamespaces,
		v1helpers.CachedSecretGetter(kubeClient.CoreV1(), kubeInformersForNamespaces),
		v1helpers.CachedConfigMapGetter(kubeClient.CoreV1(), kubeInformersForNamespaces),
		eventRecorder,
	)

	if err := AddSyncClientCertKeySecret(resourceSyncController); err != nil {
		return nil, err
	}
	return resourceSyncController, nil
}
