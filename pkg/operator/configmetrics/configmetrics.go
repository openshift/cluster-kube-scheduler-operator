package configmetrics

import (
	"github.com/prometheus/client_golang/prometheus"

	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	configlisters "github.com/openshift/client-go/config/listers/config/v1"
)

func Register(configInformer configinformers.SharedInformerFactory) {
	prometheus.MustRegister(&configMetrics{
		configLister: configInformer.Config().V1().Schedulers().Lister(),
		config: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cluster_master_schedulable",
			Help: "Reports whether the cluster master nodes are schedulable.",
		}),
	})
}

// configMetrics implements metrics gathering for this component.
type configMetrics struct {
	configLister configlisters.SchedulerLister
	config       prometheus.Gauge
}

// Describe reports the metadata for metrics to the prometheus collector.
func (m *configMetrics) Describe(ch chan<- *prometheus.Desc) {
	ch <- m.config.Desc()
}

// Collect calculates metrics from the cached config and reports them to the prometheus collector.
func (m *configMetrics) Collect(ch chan<- prometheus.Metric) {
	if config, err := m.configLister.Get("cluster"); err == nil {
		g := m.config
		if config.Spec.MastersSchedulable {
			g.Set(1)
		} else {
			g.Set(0)
		}
		ch <- g
	}
}
