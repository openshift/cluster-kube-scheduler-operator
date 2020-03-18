package configobservercontroller

import (
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	"k8s.io/client-go/tools/cache"

	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation/scheduler"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/library-go/pkg/controller/factory"

	"github.com/openshift/library-go/pkg/operator/configobserver"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
)

type ConfigObserver struct {
	factory.Controller
}

func NewConfigObserver(
	operatorClient v1helpers.OperatorClient,
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces,
	configInformer configinformers.SharedInformerFactory,
	resourceSyncer resourcesynccontroller.ResourceSyncer,
	eventRecorder events.Recorder,
) *ConfigObserver {
	interestingNamespaces := []string{
		operatorclient.GlobalUserSpecifiedConfigNamespace,
		operatorclient.GlobalMachineSpecifiedConfigNamespace,
		operatorclient.TargetNamespace,
		operatorclient.OperatorNamespace,
	}

	informers := []factory.Informer{
		operatorClient.Informer(),
		configInformer.Config().V1().Schedulers().Informer(),
	}
	for _, ns := range interestingNamespaces {
		informers = append(informers, kubeInformersForNamespaces.InformersFor(ns).Core().V1().ConfigMaps().Informer())
	}

	// TODO: This is probably not necessary anymore, please investigate deeper.
	configMapPreRunCacheSynced := []cache.InformerSynced{}
	for _, ns := range interestingNamespaces {
		configMapPreRunCacheSynced = append(configMapPreRunCacheSynced, kubeInformersForNamespaces.InformersFor(ns).Core().V1().ConfigMaps().Informer().HasSynced)
	}
	c := &ConfigObserver{
		Controller: configobserver.NewConfigObserver(
			operatorClient,
			eventRecorder,
			configobservation.Listers{
				SchedulerLister: configInformer.Config().V1().Schedulers().Lister(),
				ConfigmapLister: kubeInformersForNamespaces.ConfigMapLister(),
				ResourceSync:    resourceSyncer,
				PreRunCachesSynced: append(configMapPreRunCacheSynced,
					operatorClient.Informer().HasSynced,
					configInformer.Config().V1().Schedulers().Informer().HasSynced,
				),
			},
			informers,
			scheduler.ObserveSchedulerConfig,
		),
	}

	return c
}
