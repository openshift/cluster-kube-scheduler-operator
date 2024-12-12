// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	context "context"
	time "time"

	apiconfigv1alpha1 "github.com/openshift/api/config/v1alpha1"
	versioned "github.com/openshift/client-go/config/clientset/versioned"
	internalinterfaces "github.com/openshift/client-go/config/informers/externalversions/internalinterfaces"
	configv1alpha1 "github.com/openshift/client-go/config/listers/config/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// InsightsDataGatherInformer provides access to a shared informer and lister for
// InsightsDataGathers.
type InsightsDataGatherInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() configv1alpha1.InsightsDataGatherLister
}

type insightsDataGatherInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// NewInsightsDataGatherInformer constructs a new informer for InsightsDataGather type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewInsightsDataGatherInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredInsightsDataGatherInformer(client, resyncPeriod, indexers, nil)
}

// NewFilteredInsightsDataGatherInformer constructs a new informer for InsightsDataGather type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredInsightsDataGatherInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ConfigV1alpha1().InsightsDataGathers().List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ConfigV1alpha1().InsightsDataGathers().Watch(context.TODO(), options)
			},
		},
		&apiconfigv1alpha1.InsightsDataGather{},
		resyncPeriod,
		indexers,
	)
}

func (f *insightsDataGatherInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredInsightsDataGatherInformer(client, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *insightsDataGatherInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&apiconfigv1alpha1.InsightsDataGather{}, f.defaultInformer)
}

func (f *insightsDataGatherInformer) Lister() configv1alpha1.InsightsDataGatherLister {
	return configv1alpha1.NewInsightsDataGatherLister(f.Informer().GetIndexer())
}
