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

// Code generated by lister-gen. DO NOT EDIT.

package v1beta1

import (
	v1beta1 "github.com/saarasio/enroute/enroute-dp/apis/contour/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// TLSCertificateDelegationLister helps list TLSCertificateDelegations.
type TLSCertificateDelegationLister interface {
	// List lists all TLSCertificateDelegations in the indexer.
	List(selector labels.Selector) (ret []*v1beta1.TLSCertificateDelegation, err error)
	// TLSCertificateDelegations returns an object that can list and get TLSCertificateDelegations.
	TLSCertificateDelegations(namespace string) TLSCertificateDelegationNamespaceLister
	TLSCertificateDelegationListerExpansion
}

// tLSCertificateDelegationLister implements the TLSCertificateDelegationLister interface.
type tLSCertificateDelegationLister struct {
	indexer cache.Indexer
}

// NewTLSCertificateDelegationLister returns a new TLSCertificateDelegationLister.
func NewTLSCertificateDelegationLister(indexer cache.Indexer) TLSCertificateDelegationLister {
	return &tLSCertificateDelegationLister{indexer: indexer}
}

// List lists all TLSCertificateDelegations in the indexer.
func (s *tLSCertificateDelegationLister) List(selector labels.Selector) (ret []*v1beta1.TLSCertificateDelegation, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1beta1.TLSCertificateDelegation))
	})
	return ret, err
}

// TLSCertificateDelegations returns an object that can list and get TLSCertificateDelegations.
func (s *tLSCertificateDelegationLister) TLSCertificateDelegations(namespace string) TLSCertificateDelegationNamespaceLister {
	return tLSCertificateDelegationNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// TLSCertificateDelegationNamespaceLister helps list and get TLSCertificateDelegations.
type TLSCertificateDelegationNamespaceLister interface {
	// List lists all TLSCertificateDelegations in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1beta1.TLSCertificateDelegation, err error)
	// Get retrieves the TLSCertificateDelegation from the indexer for a given namespace and name.
	Get(name string) (*v1beta1.TLSCertificateDelegation, error)
	TLSCertificateDelegationNamespaceListerExpansion
}

// tLSCertificateDelegationNamespaceLister implements the TLSCertificateDelegationNamespaceLister
// interface.
type tLSCertificateDelegationNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all TLSCertificateDelegations in the indexer for a given namespace.
func (s tLSCertificateDelegationNamespaceLister) List(selector labels.Selector) (ret []*v1beta1.TLSCertificateDelegation, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1beta1.TLSCertificateDelegation))
	})
	return ret, err
}

// Get retrieves the TLSCertificateDelegation from the indexer for a given namespace and name.
func (s tLSCertificateDelegationNamespaceLister) Get(name string) (*v1beta1.TLSCertificateDelegation, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1beta1.Resource("tlscertificatedelegation"), name)
	}
	return obj.(*v1beta1.TLSCertificateDelegation), nil
}
