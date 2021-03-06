// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2018-2020 Saaras Inc.

/*
Copyright 2019  Heptio

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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1beta1 "github.com/saarasio/enroute/enroute-dp/apis/enroute/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeRouteFilters implements RouteFilterInterface
type FakeRouteFilters struct {
	Fake *FakeEnrouteV1beta1
	ns   string
}

var routefiltersResource = schema.GroupVersionResource{Group: "enroute.saaras.io", Version: "v1beta1", Resource: "routefilters"}

var routefiltersKind = schema.GroupVersionKind{Group: "enroute.saaras.io", Version: "v1beta1", Kind: "RouteFilter"}

// Get takes name of the routeFilter, and returns the corresponding routeFilter object, and an error if there is any.
func (c *FakeRouteFilters) Get(name string, options v1.GetOptions) (result *v1beta1.RouteFilter, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(routefiltersResource, c.ns, name), &v1beta1.RouteFilter{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.RouteFilter), err
}

// List takes label and field selectors, and returns the list of RouteFilters that match those selectors.
func (c *FakeRouteFilters) List(opts v1.ListOptions) (result *v1beta1.RouteFilterList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(routefiltersResource, routefiltersKind, c.ns, opts), &v1beta1.RouteFilterList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta1.RouteFilterList{ListMeta: obj.(*v1beta1.RouteFilterList).ListMeta}
	for _, item := range obj.(*v1beta1.RouteFilterList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested routeFilters.
func (c *FakeRouteFilters) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(routefiltersResource, c.ns, opts))

}

// Create takes the representation of a routeFilter and creates it.  Returns the server's representation of the routeFilter, and an error, if there is any.
func (c *FakeRouteFilters) Create(routeFilter *v1beta1.RouteFilter) (result *v1beta1.RouteFilter, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(routefiltersResource, c.ns, routeFilter), &v1beta1.RouteFilter{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.RouteFilter), err
}

// Update takes the representation of a routeFilter and updates it. Returns the server's representation of the routeFilter, and an error, if there is any.
func (c *FakeRouteFilters) Update(routeFilter *v1beta1.RouteFilter) (result *v1beta1.RouteFilter, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(routefiltersResource, c.ns, routeFilter), &v1beta1.RouteFilter{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.RouteFilter), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeRouteFilters) UpdateStatus(routeFilter *v1beta1.RouteFilter) (*v1beta1.RouteFilter, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(routefiltersResource, "status", c.ns, routeFilter), &v1beta1.RouteFilter{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.RouteFilter), err
}

// Delete takes name of the routeFilter and deletes it. Returns an error if one occurs.
func (c *FakeRouteFilters) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(routefiltersResource, c.ns, name), &v1beta1.RouteFilter{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeRouteFilters) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(routefiltersResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1beta1.RouteFilterList{})
	return err
}

// Patch applies the patch and returns the patched routeFilter.
func (c *FakeRouteFilters) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.RouteFilter, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(routefiltersResource, c.ns, name, pt, data, subresources...), &v1beta1.RouteFilter{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.RouteFilter), err
}
