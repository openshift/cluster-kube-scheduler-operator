package configobservercontroller

import (
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	operatorv1informers "github.com/openshift/client-go/operator/informers/externalversions"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation"
	"github.com/openshift/library-go/pkg/operator/configobserver"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
)

type ConfigObserver struct {
	*configobserver.ConfigObserver
}

func NewConfigObserver(
	operatorClient v1helpers.OperatorClient,
	operatorConfigInformers operatorv1informers.SharedInformerFactory,
	kubeInformersForOpenShiftKubeSchedulerNamespace informers.SharedInformerFactory,
	resourceSyncer resourcesynccontroller.ResourceSyncer,
	eventRecorder events.Recorder,
) *ConfigObserver {
	c := &ConfigObserver{
		ConfigObserver: configobserver.NewConfigObserver(
			operatorClient,
			eventRecorder,
			configobservation.Listers{
				ConfigmapLister: kubeInformersForOpenShiftKubeSchedulerNamespace.Core().V1().ConfigMaps().Lister(),
				ResourceSync:    resourceSyncer,
				PreRunCachesSynced: []cache.InformerSynced{
					kubeInformersForOpenShiftKubeSchedulerNamespace.Core().V1().ConfigMaps().Informer().HasSynced,
				},
			},
			nil,
		),
	}

	operatorConfigInformers.Operator().V1().KubeSchedulers().Informer().AddEventHandler(c.EventHandler())
	kubeInformersForOpenShiftKubeSchedulerNamespace.Core().V1().ConfigMaps().Informer().AddEventHandler(c.EventHandler())

	return c
}
