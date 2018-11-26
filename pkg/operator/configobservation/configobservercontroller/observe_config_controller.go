package configobservercontroller

import (
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	operatorconfiginformers "github.com/openshift/cluster-kube-scheduler-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation"
	"github.com/openshift/library-go/pkg/operator/configobserver"
)

type ConfigObserver struct {
	*configobserver.ConfigObserver
}

func NewConfigObserver(
	operatorClient configobserver.OperatorClient,
	operatorConfigInformers operatorconfiginformers.SharedInformerFactory,
	kubeInformersForOpenShiftKubeSchedulerNamespace informers.SharedInformerFactory,
) *ConfigObserver {
	c := &ConfigObserver{
		ConfigObserver: configobserver.NewConfigObserver(
			operatorClient,
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
	kubeInformersForKubeSystemNamespace.Core().V1().ConfigMaps().Informer().AddEventHandler(c.EventHandler())

	return c
}
