/*
Copyright 2019 Heptio

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by informer-gen. DO NOT EDIT.

package v1beta1

import (
	time "time"

	contourv1beta1 "github.com/saarasio/enroute/enroute-dp/apis/contour/v1beta1"
	versioned "github.com/saarasio/enroute/enroute-dp/apis/generated/clientset/versioned"
	internalinterfaces "github.com/saarasio/enroute/enroute-dp/apis/generated/informers/externalversions/internalinterfaces"
	v1beta1 "github.com/saarasio/enroute/enroute-dp/apis/generated/listers/contour/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// IngressRouteInformer provides access to a shared informer and lister for
// IngressRoutes.
type IngressRouteInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1beta1.IngressRouteLister
}

type ingressRouteInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewIngressRouteInformer constructs a new informer for IngressRoute type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewIngressRouteInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredIngressRouteInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredIngressRouteInformer constructs a new informer for IngressRoute type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredIngressRouteInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ContourV1beta1().IngressRoutes(namespace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ContourV1beta1().IngressRoutes(namespace).Watch(options)
			},
		},
		&contourv1beta1.IngressRoute{},
		resyncPeriod,
		indexers,
	)
}

func (f *ingressRouteInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredIngressRouteInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *ingressRouteInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&contourv1beta1.IngressRoute{}, f.defaultInformer)
}

func (f *ingressRouteInformer) Lister() v1beta1.IngressRouteLister {
	return v1beta1.NewIngressRouteLister(f.Informer().GetIndexer())
}
