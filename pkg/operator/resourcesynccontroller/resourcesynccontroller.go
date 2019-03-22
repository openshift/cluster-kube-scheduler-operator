package resourcesynccontroller

import (
	"k8s.io/client-go/kubernetes"

	"github.com/golang/glog"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
)

func NewResourceSyncController(
	operatorConfigClient v1helpers.OperatorClient,
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces,
	configInformer configinformers.SharedInformerFactory,
	kubeClient kubernetes.Interface,
	eventRecorder events.Recorder) (*resourcesynccontroller.ResourceSyncController, error) {

	resourceSyncController := resourcesynccontroller.NewResourceSyncController(
		operatorConfigClient,
		kubeInformersForNamespaces,
		kubeClient.CoreV1(),
		kubeClient.CoreV1(),
		eventRecorder,
	)

	scheduler, err := configInformer.Config().V1().Schedulers().Lister().Get("cluster")
	if err != nil {
		glog.Infof("Error while listing scheduler %v", err)
	}
	if scheduler != nil && len(scheduler.Spec.Policy.Name) > 0 {
		if err := resourceSyncController.SyncConfigMap(
			resourcesynccontroller.ResourceLocation{Namespace: operatorclient.TargetNamespace, Name: scheduler.Spec.Policy.Name},
			resourcesynccontroller.ResourceLocation{Namespace: operatorclient.GlobalUserSpecifiedConfigNamespace, Name: "policy-configmap"}); err != nil {
			return nil, err
		}
	}
	if err := resourceSyncController.SyncSecret(
		resourcesynccontroller.ResourceLocation{Namespace: operatorclient.TargetNamespace, Name: "kube-scheduler-client-cert-key"},
		resourcesynccontroller.ResourceLocation{Namespace: operatorclient.GlobalMachineSpecifiedConfigNamespace, Name: "kube-scheduler-client-cert-key"},
	); err != nil {
		return nil, err
	}
	return resourceSyncController, nil
}
