package resourcesynccontroller

import (
	"context"
	"github.com/openshift/library-go/pkg/operator/trace"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"

	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
)

func NewResourceSyncController(
	ctx context.Context,
	operatorConfigClient v1helpers.OperatorClient,
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces,
	configInformer configinformers.SharedInformerFactory,
	kubeClient kubernetes.Interface,
	eventRecorder events.Recorder) (*resourcesynccontroller.ResourceSyncController, error) {

	_, span := trace.TraceProvider().Tracer("resourcesynccontroller").Start(ctx, "NewResourceSyncController")
	defer span.End()

	resourceSyncController := resourcesynccontroller.NewResourceSyncController(
		operatorConfigClient,
		kubeInformersForNamespaces,
		v1helpers.CachedSecretGetter(kubeClient.CoreV1(), kubeInformersForNamespaces),
		v1helpers.CachedConfigMapGetter(kubeClient.CoreV1(), kubeInformersForNamespaces),
		eventRecorder,
	)

	scheduler, err := configInformer.Config().V1().Schedulers().Lister().Get("cluster")
	if err != nil {
		klog.Infof("Error while listing scheduler %v", err)
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
