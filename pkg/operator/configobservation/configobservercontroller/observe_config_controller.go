package configobservercontroller

import (
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	operatorconfiginformers "github.com/openshift/cluster-kube-scheduler-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation"

	"github.com/openshift/library-go/pkg/operator/configobserver"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
)

type ConfigObserver struct {
	*configobserver.ConfigObserver
}

func NewConfigObserver(
	operatorClient v1helpers.OperatorClient,
	operatorConfigInformers operatorconfiginformers.SharedInformerFactory,
	kubeInformersForOpenShiftKubeSchedulerNamespace informers.SharedInformerFactory,
	eventRecorder events.Recorder,
) *ConfigObserver {
	c := &ConfigObserver{
		ConfigObserver: configobserver.NewConfigObserver(
			operatorClient,
			eventRecorder,
			configobservation.Listers{
				ConfigmapLister: kubeInformersForOpenShiftKubeSchedulerNamespace.Core().V1().ConfigMaps().Lister(),
				PreRunCachesSynced: []cache.InformerSynced{
					kubeInformersForOpenShiftKubeSchedulerNamespace.Core().V1().ConfigMaps().Informer().HasSynced,
				},
			},
			nil,
		),
	}

	operatorConfigInformers.Kubescheduler().V1alpha1().KubeSchedulerOperatorConfigs().Informer().AddEventHandler(c.EventHandler())
	kubeInformersForOpenShiftKubeSchedulerNamespace.Core().V1().ConfigMaps().Informer().AddEventHandler(c.EventHandler())

	return c
}
