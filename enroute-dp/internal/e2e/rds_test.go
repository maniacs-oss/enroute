// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2018-2020 Saaras Inc.

// Copyright © 2018 Heptio
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// End to ends tests for translator to grpc operations.
package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	gatewayhostv1 "github.com/saarasio/enroute/enroute-dp/apis/enroute/v1beta1"
	"github.com/saarasio/enroute/enroute-dp/apis/generated/clientset/versioned/fake"
	"github.com/saarasio/enroute/enroute-dp/internal/contour"
	"github.com/saarasio/enroute/enroute-dp/internal/envoy"
	"github.com/saarasio/enroute/enroute-dp/internal/k8s"
	"github.com/saarasio/enroute/enroute-dp/internal/protobuf"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// saarasio/enroute#172. Updating an object from
//
// apiVersion: networking/v1beta1
// kind: Ingress
// metadata:
//   name: kuard
// spec:
//   backend:
//     serviceName: kuard
//     servicePort: 80
//
// to
//
// apiVersion: networking/v1beta1
// kind: Ingress
// metadata:
//   name: kuard
// spec:
//   rules:
//   - http:
//       paths:
//       - path: /testing
//         backend:
//           serviceName: kuard
//           servicePort: 80
//
// fails to update the virtualhost cache.
func TestEditIngress(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	meta := metav1.ObjectMeta{Name: "kuard", Namespace: "default"}

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	// add default/kuard to translator.
	old := &v1beta1.Ingress{
		ObjectMeta: meta,
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(80),
			},
		},
	}
	rh.OnAdd(old)

	// check that it's been translated correctly.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []*envoy_api_v2_route.Route{{
						Match:               envoy.RouteMatch("/"),
						Action:              routecluster("default/kuard/80/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}},
				}},
			},
			&v2.RouteConfiguration{
				Name: "ingress_https",
			},
		),
		TypeUrl: routeType,
		Nonce:   "2",
	}, streamRDS(t, cc))

	// update old to new
	rh.OnUpdate(old, &v1beta1.Ingress{
		ObjectMeta: meta,
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/testing",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	})

	// check that ingress_http has been updated.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "3",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []*envoy_api_v2_route.Route{{
						Match:               envoy.RouteMatch("/testing"),
						Action:              routecluster("default/kuard/80/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}},
				}},
			},
			&v2.RouteConfiguration{
				Name: "ingress_https",
			},
		),
		TypeUrl: routeType,
		Nonce:   "3",
	}, streamRDS(t, cc))
}

// saarasio/enroute#101
// The path /hello should point to default/hello/80 on "*"
//
// apiVersion: networking/v1beta1
// kind: Ingress
// metadata:
//   name: hello
// spec:
//   rules:
//   - http:
// 	 paths:
//       - path: /hello
//         backend:
//           serviceName: hello
//           servicePort: 80
func TestIngressPathRouteWithoutHost(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// add default/hello to translator.
	rh.OnAdd(&v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default"},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/hello",
							Backend: v1beta1.IngressBackend{
								ServiceName: "hello",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	})

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	// check that it's been translated correctly.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []*envoy_api_v2_route.Route{{
						Match:               envoy.RouteMatch("/hello"),
						Action:              routecluster("default/hello/80/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}},
				}},
			},
			&v2.RouteConfiguration{
				Name: "ingress_https",
			},
		),
		TypeUrl: routeType,
		Nonce:   "2",
	}, streamRDS(t, cc))
}

func TestEditIngressInPlace(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default"},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "hello.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "wowie",
								ServicePort: intstr.FromString("http"),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wowie",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kerpow",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       9000,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s2)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "3",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "hello.example.com",
					Domains: domains("hello.example.com"),
					Routes: []*envoy_api_v2_route.Route{{
						Match:               envoy.RouteMatch("/"),
						Action:              routecluster("default/wowie/80/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}},
				}},
			},
			&v2.RouteConfiguration{
				Name: "ingress_https",
			},
		),
		TypeUrl: routeType,
		Nonce:   "3",
	}, streamRDS(t, cc))

	// i2 is like i1 but adds a second route
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default"},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "hello.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "wowie",
								ServicePort: intstr.FromInt(80),
							},
						}, {
							Path: "/whoop",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kerpow",
								ServicePort: intstr.FromInt(9000),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnUpdate(i1, i2)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "4",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "hello.example.com",
					Domains: domains("hello.example.com"),
					Routes: []*envoy_api_v2_route.Route{{
						Match:               envoy.RouteMatch("/whoop"),
						Action:              routecluster("default/kerpow/9000/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}, {
						Match:               envoy.RouteMatch("/"),
						Action:              routecluster("default/wowie/80/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}},
				}},
			},
			&v2.RouteConfiguration{
				Name: "ingress_https",
			},
		),
		TypeUrl: routeType,
		Nonce:   "4",
	}, streamRDS(t, cc))

	// i3 is like i2, but adds the ingress.kubernetes.io/force-ssl-redirect: "true" annotation
	i3 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true"},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "hello.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "wowie",
								ServicePort: intstr.FromInt(80),
							},
						}, {
							Path: "/whoop",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kerpow",
								ServicePort: intstr.FromInt(9000),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnUpdate(i2, i3)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "5",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "hello.example.com",
					Domains: domains("hello.example.com"),
					Routes: []*envoy_api_v2_route.Route{{
						Match:  envoy.RouteMatch("/whoop"),
						Action: envoy.UpgradeHTTPS(),
					}, {
						Match:  envoy.RouteMatch("/"),
						Action: envoy.UpgradeHTTPS(),
					}},
				}},
			},
			&v2.RouteConfiguration{Name: "ingress_https"},
		),
		TypeUrl: routeType,
		Nonce:   "5",
	}, streamRDS(t, cc))

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello-kitty",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
	})

	// i4 is the same as i3, and includes a TLS spec object to enable ingress_https routes
	// i3 is like i2, but adds the ingress.kubernetes.io/force-ssl-redirect: "true" annotation
	i4 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true"},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"hello.example.com"},
				SecretName: "hello-kitty",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "hello.example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "wowie",
								ServicePort: intstr.FromInt(80),
							},
						}, {
							Path: "/whoop",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kerpow",
								ServicePort: intstr.FromInt(9000),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnUpdate(i3, i4)
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "7",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "hello.example.com",
					Domains: domains("hello.example.com"),
					Routes: []*envoy_api_v2_route.Route{{
						Match:  envoy.RouteMatch("/whoop"),
						Action: envoy.UpgradeHTTPS(),
					}, {
						Match:  envoy.RouteMatch("/"),
						Action: envoy.UpgradeHTTPS(),
					}},
				}},
			},
			&v2.RouteConfiguration{
				Name: "ingress_https",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "hello.example.com",
					Domains: domains("hello.example.com"),
					Routes: []*envoy_api_v2_route.Route{{
						Match:               envoy.RouteMatch("/whoop"),
						Action:              routecluster("default/kerpow/9000/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}, {
						Match:               envoy.RouteMatch("/"),
						Action:              routecluster("default/wowie/80/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}},
				}},
			},
		),
		TypeUrl: routeType,
		Nonce:   "7",
	}, streamRDS(t, cc))
}

// contour#164: backend request timeout support
func TestRequestTimeout(t *testing.T) {
	const (
		durationInfinite  = time.Duration(0)
		duration10Minutes = 10 * time.Minute
	)

	rh, cc, done := setup(t)
	defer done()

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	// i1 is a simple ingress bound to the default vhost.
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default"},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}
	rh.OnAdd(i1)
	assertRDS(t, cc, "2", []*envoy_api_v2_route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              routecluster("default/backend/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	// i2 adds an _invalid_ timeout, which we interpret as _infinite_.
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default",
			Annotations: map[string]string{
				"enroute.saaras.io/request-timeout": "600", // not valid
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}
	rh.OnUpdate(i1, i2)
	assertRDS(t, cc, "3", []*envoy_api_v2_route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              clustertimeout("default/backend/80/da39a3ee5e", durationInfinite),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	// i3 corrects i2 to use a proper duration
	i3 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default",
			Annotations: map[string]string{
				"enroute.saaras.io/request-timeout": "600s", // 10 * time.Minute
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}
	rh.OnUpdate(i2, i3)
	assertRDS(t, cc, "4", []*envoy_api_v2_route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              clustertimeout("default/backend/80/da39a3ee5e", duration10Minutes),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	// i4 updates i3 to explicitly request infinite timeout
	i4 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default",
			Annotations: map[string]string{
				"enroute.saaras.io/request-timeout": "infinity",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}
	rh.OnUpdate(i3, i4)
	assertRDS(t, cc, "5", []*envoy_api_v2_route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              clustertimeout("default/backend/80/da39a3ee5e", durationInfinite),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)
}

// contour#250 ingress.kubernetes.io/force-ssl-redirect: "true" should apply
// per route, not per vhost.
func TestSSLRedirectOverlay(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// i1 is a stock ingress with force-ssl-redirect on the / route
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: "example-tls",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "app-service",
								ServicePort: intstr.FromInt(8080),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
	})

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-service",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	// i2 is an overlay to add the let's encrypt handler.
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "challenge", Namespace: "nginx-ingress"},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk",
							Backend: v1beta1.IngressBackend{
								ServiceName: "challenge-service",
								ServicePort: intstr.FromInt(8009),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i2)

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "challenge-service",
			Namespace: "nginx-ingress",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8009,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	assertRDS(t, cc, "5", []*envoy_api_v2_route.VirtualHost{{ // ingress_http
		Name:    "example.com",
		Domains: domains("example.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
			Action:              routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}, {
			Match:  envoy.RouteMatch("/"), // match all
			Action: envoy.UpgradeHTTPS(),
		}},
	}}, []*envoy_api_v2_route.VirtualHost{{ // ingress_https
		Name:    "example.com",
		Domains: domains("example.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
			Action:              routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}, {
			Match:               envoy.RouteMatch("/"), // match all
			Action:              routecluster("default/app-service/8080/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}})
}

func TestInvalidCertInIngress(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// Create an invalid TLS secret
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: map[string][]byte{
			v1.TLSCertKey:       nil,
			v1.TLSPrivateKeyKey: []byte("key"),
		},
	}
	rh.OnAdd(secret)

	// Create a service
	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	// Create an ingress that uses the invalid secret
	rh.OnAdd(&v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "kuard-ing", Namespace: "default"},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"kuard.io"},
				SecretName: "example-tls",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.io",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	})

	assertRDS(t, cc, "3", []*envoy_api_v2_route.VirtualHost{{ // ingress_http
		Name:    "kuard.io",
		Domains: domains("kuard.io"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"),
			Action:              routecluster("default/kuard/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	// Correct the secret
	rh.OnUpdate(secret, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("cert"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
	})

	assertRDS(t, cc, "4", []*envoy_api_v2_route.VirtualHost{{ // ingress_http
		Name:    "kuard.io",
		Domains: domains("kuard.io"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"),
			Action:              routecluster("default/kuard/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, []*envoy_api_v2_route.VirtualHost{{ // ingress_https
		Name:    "kuard.io",
		Domains: domains("kuard.io"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"),
			Action:              routecluster("default/kuard/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}})
}

// issue #257: editing default ingress did not remove original default route
func TestIssue257(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// apiVersion: networking/v1beta1
	// kind: Ingress
	// metadata:
	//   name: kuard-ing
	//   labels:
	//     app: kuard
	//   annotations:
	//     kubernetes.io/ingress.class: contour
	// spec:
	//   backend:
	//     serviceName: kuard
	//     servicePort: 80
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "contour",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(80),
			},
		},
	}
	rh.OnAdd(i1)

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	assertRDS(t, cc, "2", []*envoy_api_v2_route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              routecluster("default/kuard/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	// apiVersion: networking/v1beta1
	// kind: Ingress
	// metadata:
	//   name: kuard-ing
	//   labhls:
	//     app: kuard
	//   annotations:
	//     kubernetes.io/ingress.class: contour
	// spec:
	//  rules:
	//  - host: kuard.db.gd-ms.com
	//    http:
	//      paths:
	//      - backend:
	//         serviceName: kuard
	//         servicePort: 80
	//        path: /
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "contour",
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "kuard.db.gd-ms.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnUpdate(i1, i2)

	assertRDS(t, cc, "3", []*envoy_api_v2_route.VirtualHost{{
		Name:    "kuard.db.gd-ms.com",
		Domains: domains("kuard.db.gd-ms.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              routecluster("default/kuard/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)
}

func TestRDSFilter(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// i1 is a stock ingress with force-ssl-redirect on the / route
	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app",
			Namespace: "default",
			Annotations: map[string]string{
				"ingress.kubernetes.io/force-ssl-redirect": "true",
			},
		},
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{{
				Hosts:      []string{"example.com"},
				SecretName: "example-tls",
			}},
			Rules: []v1beta1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "app-service",
								ServicePort: intstr.FromInt(8080),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
	})

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-service",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	// i2 is an overlay to add the let's encrypt handler.
	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "challenge", Namespace: "nginx-ingress"},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk",
							Backend: v1beta1.IngressBackend{
								ServiceName: "challenge-service",
								ServicePort: intstr.FromInt(8009),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i2)

	s2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "challenge-service",
			Namespace: "nginx-ingress",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8009,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s2)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "5",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{ // ingress_http
					Name:    "example.com",
					Domains: domains("example.com"),
					Routes: []*envoy_api_v2_route.Route{{
						Match:               envoy.RouteMatch("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
						Action:              routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}, {
						Match:  envoy.RouteMatch("/"), // match all
						Action: envoy.UpgradeHTTPS(),
					}},
				}},
			},
		),
		TypeUrl: routeType,
		Nonce:   "5",
	}, streamRDS(t, cc, "ingress_http"))

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "5",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_https",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{ // ingress_https
					Name:    "example.com",
					Domains: domains("example.com"),
					Routes: []*envoy_api_v2_route.Route{{
						Match:               envoy.RouteMatch("/.well-known/acme-challenge/gVJl5NWL2owUqZekjHkt_bo3OHYC2XNDURRRgLI5JTk"),
						Action:              routecluster("nginx-ingress/challenge-service/8009/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}, {
						Match:               envoy.RouteMatch("/"), // match all
						Action:              routecluster("default/app-service/8080/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}},
				}},
			},
		),
		TypeUrl: routeType,
		Nonce:   "5",
	}, streamRDS(t, cc, "ingress_https"))
}

func TestWebsocketIngress(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
			Annotations: map[string]string{
				"enroute.saaras.io/websocket-routes": "/",
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "websocket.hello.world",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "ws",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	})

	assertRDS(t, cc, "2", []*envoy_api_v2_route.VirtualHost{{
		Name:    "websocket.hello.world",
		Domains: domains("websocket.hello.world"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              websocketroute("default/ws/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)
}

func TestWebsocketGatewayHost(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "websocket.hello.world"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				Services: []gatewayhostv1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}, {
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/ws-1",
				}},
				EnableWebsockets: true,
				Services: []gatewayhostv1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}, {
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/ws-2",
				}},
				EnableWebsockets: true,
				Services: []gatewayhostv1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}},
		},
	})

	assertRDS(t, cc, "2", []*envoy_api_v2_route.VirtualHost{{
		Name:    "websocket.hello.world",
		Domains: domains("websocket.hello.world"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/ws-2"),
			Action:              websocketroute("default/ws/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}, {
			Match:               envoy.RouteMatch("/ws-1"),
			Action:              websocketroute("default/ws/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}, {
			Match:               envoy.RouteMatch("/"), // match all
			Action:              routecluster("default/ws/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)
}
func TestPrefixRewriteGatewayHost(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "prefixrewrite.hello.world"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				Services: []gatewayhostv1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}, {
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/ws-1",
				}},
				PrefixRewrite: "/",
				Services: []gatewayhostv1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}, {
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/ws-2",
				}},
				PrefixRewrite: "/",
				Services: []gatewayhostv1.Service{{
					Name: "ws",
					Port: 80,
				}},
			}},
		},
	})

	assertRDS(t, cc, "2", []*envoy_api_v2_route.VirtualHost{{
		Name:    "prefixrewrite.hello.world",
		Domains: domains("prefixrewrite.hello.world"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/ws-2"),
			Action:              prefixrewriteroute("default/ws/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}, {
			Match:               envoy.RouteMatch("/ws-1"),
			Action:              prefixrewriteroute("default/ws/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}, {
			Match:               envoy.RouteMatch("/"), // match all
			Action:              routecluster("default/ws/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)
}

// issue 404
func TestDefaultBackendDoesNotOverwriteNamedHost(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}, {
				Name:       "alt",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gui",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(80),
			},

			Rules: []v1beta1.IngressRule{{
				Host: "test-gui",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "test-gui",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}, {
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/kuard",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromInt(8080),
							},
						}},
					},
				},
			}},
		},
	})

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "3",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []*envoy_api_v2_route.Route{{
						Match:               envoy.RouteMatch("/kuard"),
						Action:              routecluster("default/kuard/8080/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}, {
						Match:               envoy.RouteMatch("/"),
						Action:              routecluster("default/kuard/80/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}},
				}, {
					Name:    "test-gui",
					Domains: domains("test-gui"),
					Routes: []*envoy_api_v2_route.Route{{
						Match:               envoy.RouteMatch("/"),
						Action:              routecluster("default/test-gui/80/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}},
				}},
			},
		),
		TypeUrl: routeType,
		Nonce:   "3",
	}, streamRDS(t, cc, "ingress_http"))
}

func TestRDSGatewayHostInsideRootNamespaces(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.ResourceEventHandler) {
		reh.GatewayHostRootNamespaces = []string{"roots"}
		reh.Notifier.(*contour.CacheHandler).GatewayHostStatus = &k8s.GatewayHostStatus{
			Client: fake.NewSimpleClientset(),
		}
	})
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "roots",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	// ir1 is an gatewayhost that is in the root namespaces
	ir1 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "roots",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "example.com"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				Services: []gatewayhostv1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// add gatewayhost
	rh.OnAdd(ir1)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "example.com",
					Domains: domains("example.com"),
					Routes: []*envoy_api_v2_route.Route{{
						Match:               envoy.RouteMatch("/"),
						Action:              routecluster("roots/kuard/8080/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}},
				}},
			},
		),
		TypeUrl: routeType,
		Nonce:   "2",
	}, streamRDS(t, cc, "ingress_http"))
}

func TestRDSGatewayHostOutsideRootNamespaces(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.ResourceEventHandler) {
		reh.GatewayHostRootNamespaces = []string{"roots"}
		reh.Notifier.(*contour.CacheHandler).GatewayHostStatus = &k8s.GatewayHostStatus{
			Client: fake.NewSimpleClientset(),
		}
	})
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	// ir1 is an gatewayhost that is not in the root namespaces
	ir1 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "example.com"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				Services: []gatewayhostv1.Service{{
					Name: "kuard",
					Port: 8080,
				}},
			}},
		},
	}

	// add gatewayhost
	rh.OnAdd(ir1)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "2",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_http",
			}),
		TypeUrl: routeType,
		Nonce:   "2",
	}, streamRDS(t, cc, "ingress_http"))
}

// Test DAGAdapter.IngressClass setting works, this could be done
// in LDS or RDS, or even CDS, but this test mirrors the place it's
// tested in internal/contour/route_test.go
func TestRDSGatewayHostClassAnnotation(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.ResourceEventHandler) {
		reh.IngressClass = "linkerd"
	})
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	ir1 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard ",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				Services: []gatewayhostv1.Service{
					{
						Name: "kuard",
						Port: 8080,
					},
				},
			}},
		},
	}

	rh.OnAdd(ir1)
	assertRDS(t, cc, "2", []*envoy_api_v2_route.VirtualHost{{
		Name:    "www.example.com",
		Domains: domains("www.example.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"),
			Action:              routecluster("default/kuard/8080/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	ir2 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard ",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "contour",
			},
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				Services: []gatewayhostv1.Service{
					{
						Name: "kuard",
						Port: 8080,
					},
				},
			}},
		},
	}
	rh.OnUpdate(ir1, ir2)
	assertRDS(t, cc, "3", nil, nil)

	ir3 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard ",
			Namespace: "default",
			Annotations: map[string]string{
				"enroute.saaras.io/ingress.class": "contour",
			},
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				Services: []gatewayhostv1.Service{
					{
						Name: "kuard",
						Port: 8080,
					},
				},
			}},
		},
	}
	rh.OnUpdate(ir2, ir3)
	assertRDS(t, cc, "3", nil, nil)

	ir4 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard ",
			Namespace: "default",
			Annotations: map[string]string{
				"enroute.saaras.io/ingress.class": "linkerd",
			},
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				Services: []gatewayhostv1.Service{
					{
						Name: "kuard",
						Port: 8080,
					},
				},
			}},
		},
	}
	rh.OnUpdate(ir3, ir4)
	assertRDS(t, cc, "4", []*envoy_api_v2_route.VirtualHost{{
		Name:    "www.example.com",
		Domains: domains("www.example.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"),
			Action:              routecluster("default/kuard/8080/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	ir5 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard ",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "linkerd",
			},
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{
				Fqdn: "www.example.com",
			},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				Services: []gatewayhostv1.Service{
					{
						Name: "kuard",
						Port: 8080,
					},
				},
			}},
		},
	}
	rh.OnUpdate(ir4, ir5)

	assertRDS(t, cc, "5", []*envoy_api_v2_route.VirtualHost{{
		Name:    "www.example.com",
		Domains: domains("www.example.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"),
			Action:              routecluster("default/kuard/8080/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	rh.OnUpdate(ir5, ir3)
	assertRDS(t, cc, "6", nil, nil)
}

// Test DAGAdapter.IngressClass setting works, this could be done
// in LDS or RDS, or even CDS, but this test mirrors the place it's
// tested in internal/contour/route_test.go
func TestRDSIngressClassAnnotation(t *testing.T) {
	rh, cc, done := setup(t, func(reh *contour.ResourceEventHandler) {
		reh.IngressClass = "linkerd"
	})
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(8080),
			},
		},
	}
	rh.OnAdd(i1)
	assertRDS(t, cc, "2", []*envoy_api_v2_route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"),
			Action:              routecluster("default/kuard/8080/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	i2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "contour",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(8080),
			},
		},
	}
	rh.OnUpdate(i1, i2)
	assertRDS(t, cc, "3", nil, nil)

	i3 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"enroute.saaras.io/ingress.class": "contour",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(8080),
			},
		},
	}
	rh.OnUpdate(i2, i3)
	assertRDS(t, cc, "3", nil, nil)

	i4 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "linkerd",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(8080),
			},
		},
	}
	rh.OnUpdate(i3, i4)
	assertRDS(t, cc, "4", []*envoy_api_v2_route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"),
			Action:              routecluster("default/kuard/8080/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	i5 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard-ing",
			Namespace: "default",
			Annotations: map[string]string{
				"enroute.saaras.io/ingress.class": "linkerd",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(8080),
			},
		},
	}
	rh.OnUpdate(i4, i5)
	assertRDS(t, cc, "5", []*envoy_api_v2_route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"),
			Action:              routecluster("default/kuard/8080/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	rh.OnUpdate(i5, i3)
	assertRDS(t, cc, "6", nil, nil)
}

// issue 523, check for data races caused by accidentally
// sorting the contents of an RDS entry's virtualhost list.
func TestRDSAssertNoDataRaceDuringInsertAndStream(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	stop := make(chan struct{})

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	go func() {
		for i := 0; i < 100; i++ {
			rh.OnAdd(&gatewayhostv1.GatewayHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("simple-%d", i),
					Namespace: "default",
				},
				Spec: gatewayhostv1.GatewayHostSpec{
					VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: fmt.Sprintf("example-%d.com", i)},
					Routes: []gatewayhostv1.Route{{
						Conditions: []gatewayhostv1.Condition{{
							Prefix: "/",
						}},
						Services: []gatewayhostv1.Service{{
							Name: "kuard",
							Port: 80,
						}},
					}},
				},
			})
		}
		close(stop)
	}()

	for {
		select {
		case <-stop:
			return
		default:
			streamRDS(t, cc)
		}
	}
}

// issue 606: spec.rules.host without a http key causes panic.
// apiVersion: networking/v1beta1
// kind: Ingress
// metadata:
//   name: test-ingress3
// spec:
//   rules:
//   - host: test1.test.com
//   - host: test2.test.com
//     http:
//       paths:
//       - backend:
//           serviceName: network-test
//           servicePort: 9001
//         path: /
//
// note: this test caused a panic in dag.Builder, but testing the
// context of RDS is a good place to start.
func TestRDSIngressSpecMissingHTTPKey(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress3",
			Namespace: "default",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "test1.test.com",
			}, {
				Host: "test2.test.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/",
							Backend: v1beta1.IngressBackend{
								ServiceName: "network-test",
								ServicePort: intstr.FromInt(9001),
							},
						}},
					},
				},
			}},
		},
	}
	rh.OnAdd(i1)

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "network-test",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:       "http",
				Protocol:   "TCP",
				Port:       9001,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	assertRDS(t, cc, "2", []*envoy_api_v2_route.VirtualHost{{
		Name:    "test2.test.com",
		Domains: domains("test2.test.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              routecluster("default/network-test/9001/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)
}

func TestRouteWithAServiceWeight(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	ir1 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/a",
				}},
				Services: []gatewayhostv1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90, // ignored
				}},
			}},
		},
	}

	rh.OnAdd(ir1)
	assertRDS(t, cc, "2", []*envoy_api_v2_route.VirtualHost{{
		Name:    "test2.test.com",
		Domains: domains("test2.test.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/a"), // match all
			Action:              routecluster("default/kuard/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	ir2 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/a",
				}},
				Services: []gatewayhostv1.Service{{
					Name:   "kuard",
					Port:   80,
					Weight: 90,
				}, {
					Name:   "kuard",
					Port:   80,
					Weight: 60,
				}},
			}},
		},
	}

	rh.OnUpdate(ir1, ir2)
	assertRDS(t, cc, "3", []*envoy_api_v2_route.VirtualHost{{
		Name:    "test2.test.com",
		Domains: domains("test2.test.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match: envoy.RouteMatch("/a"), // match all
			Action: routeweightedcluster(
				weightedcluster{"default/kuard/80/da39a3ee5e", 60},
				weightedcluster{"default/kuard/80/da39a3ee5e", 90},
			),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)
}

func TestRouteWithTLS(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
	})

	ir1 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &gatewayhostv1.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/a",
				}},
				Services: []gatewayhostv1.Service{{
					Name: "kuard",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(ir1)

	// check that ingress_http has been updated.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "3",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "test2.test.com",
					Domains: domains("test2.test.com"),
					Routes: []*envoy_api_v2_route.Route{{
						Match:  envoy.RouteMatch("/a"),
						Action: envoy.UpgradeHTTPS(),
					}},
				}}},
			&v2.RouteConfiguration{
				Name: "ingress_https",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "test2.test.com",
					Domains: domains("test2.test.com"),
					Routes: []*envoy_api_v2_route.Route{{
						Match:               envoy.RouteMatch("/a"),
						Action:              routecluster("default/kuard/80/da39a3ee5e"),
						RequestHeadersToAdd: envoy.RouteHeaders(),
					}},
				}}},
		),
		TypeUrl: routeType,
		Nonce:   "3",
	}, streamRDS(t, cc))
}
func TestRouteWithTLS_InsecurePaths(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kuard",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc2",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	rh.OnAdd(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-tls",
			Namespace: "default",
		},
		Type: "kubernetes.io/tls",
		Data: map[string][]byte{
			v1.TLSCertKey:       []byte("certificate"),
			v1.TLSPrivateKeyKey: []byte("key"),
		},
	})

	ir1 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{
				Fqdn: "test2.test.com",
				TLS: &gatewayhostv1.TLS{
					SecretName: "example-tls",
				},
			},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/insecure",
				}},
				PermitInsecure: true,
				Services: []gatewayhostv1.Service{{Name: "kuard",
					Port: 80,
				}},
			}, {
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/secure",
				}},
				Services: []gatewayhostv1.Service{{
					Name: "svc2",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(ir1)

	// check that ingress_http has been updated.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "4",
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "test2.test.com",
					Domains: domains("test2.test.com"),
					Routes: []*envoy_api_v2_route.Route{
						{
							Match:  envoy.RouteMatch("/secure"),
							Action: envoy.UpgradeHTTPS(),
						}, {
							Match:               envoy.RouteMatch("/insecure"),
							Action:              routecluster("default/kuard/80/da39a3ee5e"),
							RequestHeadersToAdd: envoy.RouteHeaders(),
						},
					},
				}}},
			&v2.RouteConfiguration{
				Name: "ingress_https",
				VirtualHosts: []*envoy_api_v2_route.VirtualHost{{
					Name:    "test2.test.com",
					Domains: domains("test2.test.com"),
					Routes: []*envoy_api_v2_route.Route{
						{
							Match:               envoy.RouteMatch("/secure"),
							Action:              routecluster("default/svc2/80/da39a3ee5e"),
							RequestHeadersToAdd: envoy.RouteHeaders(),
						}, {
							Match:               envoy.RouteMatch("/insecure"),
							Action:              routecluster("default/kuard/80/da39a3ee5e"),
							RequestHeadersToAdd: envoy.RouteHeaders(),
						},
					},
				}}},
		),
		TypeUrl: routeType,
		Nonce:   "4",
	}, streamRDS(t, cc))
}

// issue 665, support for retry-on, num-retries, and per-try-timeout annotations.
func TestRouteRetryAnnotations(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	i1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hello", Namespace: "default",
			Annotations: map[string]string{
				"enroute.saaras.io/retry-on":        "5xx,gateway-error",
				"enroute.saaras.io/num-retries":     "7",
				"enroute.saaras.io/per-try-timeout": "120ms",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: backend("backend", intstr.FromInt(80)),
		},
	}
	rh.OnAdd(i1)
	assertRDS(t, cc, "2", []*envoy_api_v2_route.VirtualHost{{
		Name:    "*",
		Domains: []string{"*"},
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              routeretry("default/backend/80/da39a3ee5e", "5xx,gateway-error", 7, 120*time.Millisecond),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)
}

// issue 815, support for retry-on, num-retries, and per-try-timeout in GatewayHost
func TestRouteRetryGatewayHost(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	i1 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				RetryPolicy: &gatewayhostv1.RetryPolicy{
					NumRetries:    7,
					PerTryTimeout: "120ms",
				},
				Services: []gatewayhostv1.Service{{
					Name: "backend",
					Port: 80,
				}},
			}},
		},
	}

	rh.OnAdd(i1)
	assertRDS(t, cc, "2", []*envoy_api_v2_route.VirtualHost{{
		Name:    "test2.test.com",
		Domains: domains("test2.test.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              routeretry("default/backend/80/da39a3ee5e", "5xx", 7, 120*time.Millisecond),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)
}

// issue 815, support for timeoutpolicy in GatewayHost
func TestRouteTimeoutPolicyGatewayHost(t *testing.T) {
	const (
		durationInfinite  = time.Duration(0)
		duration10Minutes = 10 * time.Minute
	)

	rh, cc, done := setup(t)
	defer done()

	s1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	rh.OnAdd(s1)

	// i1 is an _invalid_ timeout, which we interpret as _infinite_.
	i1 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				Services: []gatewayhostv1.Service{{
					Name: "backend",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnAdd(i1)
	assertRDS(t, cc, "2", []*envoy_api_v2_route.VirtualHost{{
		Name:    "test2.test.com",
		Domains: domains("test2.test.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              routecluster("default/backend/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	// i2 adds an _invalid_ timeout, which we interpret as _infinite_.
	i2 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				TimeoutPolicy: &gatewayhostv1.TimeoutPolicy{
					Request: "600",
				},
				Services: []gatewayhostv1.Service{{
					Name: "backend",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnUpdate(i1, i2)
	assertRDS(t, cc, "3", []*envoy_api_v2_route.VirtualHost{{
		Name:    "test2.test.com",
		Domains: domains("test2.test.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              clustertimeout("default/backend/80/da39a3ee5e", durationInfinite),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)
	// i3 corrects i2 to use a proper duration
	i3 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				TimeoutPolicy: &gatewayhostv1.TimeoutPolicy{
					Request: "600s", // 10 * time.Minute
				},
				Services: []gatewayhostv1.Service{{
					Name: "backend",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnUpdate(i2, i3)
	assertRDS(t, cc, "4", []*envoy_api_v2_route.VirtualHost{{
		Name:    "test2.test.com",
		Domains: domains("test2.test.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              clustertimeout("default/backend/80/da39a3ee5e", duration10Minutes),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)
	// i4 updates i3 to explicitly request infinite timeout
	i4 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				TimeoutPolicy: &gatewayhostv1.TimeoutPolicy{
					Request: "infinity",
				},
				Services: []gatewayhostv1.Service{{
					Name: "backend",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnUpdate(i3, i4)
	assertRDS(t, cc, "5", []*envoy_api_v2_route.VirtualHost{{
		Name:    "test2.test.com",
		Domains: domains("test2.test.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/"), // match all
			Action:              clustertimeout("default/backend/80/da39a3ee5e", durationInfinite),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)
}

func TestRouteWithSessionAffinity(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}, {
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	})

	// simple single service
	ir1 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/cart",
				}},
				Services: []gatewayhostv1.Service{{
					Name:     "app",
					Port:     80,
					Strategy: "Cookie",
				}},
			}},
		},
	}

	rh.OnAdd(ir1)
	assertRDS(t, cc, "2", []*envoy_api_v2_route.VirtualHost{{
		Name:    "www.example.com",
		Domains: domains("www.example.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match:               envoy.RouteMatch("/cart"),
			Action:              withSessionAffinity(routecluster("default/app/80/e4f81994fe")),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	// two backends
	ir2 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/cart",
				}},
				Services: []gatewayhostv1.Service{{
					Name:     "app",
					Port:     80,
					Strategy: "Cookie",
				}, {
					Name:     "app",
					Port:     8080,
					Strategy: "Cookie",
				}},
			}},
		},
	}
	rh.OnUpdate(ir1, ir2)
	assertRDS(t, cc, "3", []*envoy_api_v2_route.VirtualHost{{
		Name:    "www.example.com",
		Domains: domains("www.example.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match: envoy.RouteMatch("/cart"),
			Action: withSessionAffinity(
				routeweightedcluster(
					weightedcluster{"default/app/80/e4f81994fe", 1},
					weightedcluster{"default/app/8080/e4f81994fe", 1},
				),
			),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

	// two mixed backends
	ir3 := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "www.example.com"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/cart",
				}},
				Services: []gatewayhostv1.Service{{
					Name:     "app",
					Port:     80,
					Strategy: "Cookie",
				}, {
					Name: "app",
					Port: 8080,
				}},
			}, {
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/",
				}},
				Services: []gatewayhostv1.Service{{
					Name: "app",
					Port: 80,
				}},
			}},
		},
	}
	rh.OnUpdate(ir2, ir3)
	assertRDS(t, cc, "4", []*envoy_api_v2_route.VirtualHost{{
		Name:    "www.example.com",
		Domains: domains("www.example.com"),
		Routes: []*envoy_api_v2_route.Route{{
			Match: envoy.RouteMatch("/cart"),
			Action: withSessionAffinity(
				routeweightedcluster(
					weightedcluster{"default/app/80/e4f81994fe", 1},
					weightedcluster{"default/app/8080/da39a3ee5e", 1},
				),
			),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}, {
			Match:               envoy.RouteMatch("/"),
			Action:              routecluster("default/app/80/da39a3ee5e"),
			RequestHeadersToAdd: envoy.RouteHeaders(),
		}},
	}}, nil)

}

// issue 681 Increase the e2e coverage of lb strategies
func TestLoadBalancingStrategies(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	st := v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "template",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	services := []struct {
		name       string
		lbHash     string
		lbStrategy string
		lbDesc     string
	}{
		{"s1", "f3b72af6a9", "RoundRobin", "RoundRobin lb algorithm"},
		{"s2", "8bf87fefba", "WeightedLeastRequest", "WeightedLeastRequest lb algorithm"},
		{"s5", "58d888c08a", "Random", "Random lb algorithm"},
		{"s6", "da39a3ee5e", "", "Default lb algorithm"},
	}
	ss := make([]gatewayhostv1.Service, len(services))
	wc := make([]weightedcluster, len(services))
	for i, x := range services {
		s := st
		s.ObjectMeta.Name = x.name
		rh.OnAdd(&s)
		ss[i] = gatewayhostv1.Service{
			Name:     x.name,
			Port:     80,
			Strategy: x.lbStrategy,
		}
		wc[i] = weightedcluster{fmt.Sprintf("default/%s/80/%s", x.name, x.lbHash), 1}
	}

	ir := &gatewayhostv1.GatewayHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple",
			Namespace: "default",
		},
		Spec: gatewayhostv1.GatewayHostSpec{
			VirtualHost: &gatewayhostv1.VirtualHost{Fqdn: "test2.test.com"},
			Routes: []gatewayhostv1.Route{{
				Conditions: []gatewayhostv1.Condition{{
					Prefix: "/a",
				}},
				Services: ss,
			}},
		},
	}

	rh.OnAdd(ir)
	want := []*envoy_api_v2_route.VirtualHost{{
		Name:    "test2.test.com",
		Domains: domains("test2.test.com"),
		Routes: []*envoy_api_v2_route.Route{
			{
				Match:               envoy.RouteMatch("/a"),
				Action:              routeweightedcluster(wc...),
				RequestHeadersToAdd: envoy.RouteHeaders(),
			},
		},
	}}
	assertRDS(t, cc, "5", want, nil)
}

func assertRDS(t *testing.T, cc *grpc.ClientConn, versioninfo string, ingress_http, ingress_https []*envoy_api_v2_route.VirtualHost) {
	t.Helper()
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: versioninfo,
		Resources: resources(t,
			&v2.RouteConfiguration{
				Name:         "ingress_http",
				VirtualHosts: ingress_http,
			},
			&v2.RouteConfiguration{
				Name:         "ingress_https",
				VirtualHosts: ingress_https,
			},
		),
		TypeUrl: routeType,
		Nonce:   versioninfo,
	}, streamRDS(t, cc))
}

func domains(hostname string) []string {
	if hostname == "*" {
		return []string{"*"}
	}
	return []string{hostname, hostname + ":*"}
}

func streamRDS(t *testing.T, cc *grpc.ClientConn, rn ...string) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewRouteDiscoveryServiceClient(cc)
	st, err := rds.StreamRoutes(context.TODO())
	check(t, err)
	return stream(t, st, &v2.DiscoveryRequest{
		TypeUrl:       routeType,
		ResourceNames: rn,
	})
}

type weightedcluster struct {
	name   string
	weight uint32
}

func withSessionAffinity(r *envoy_api_v2_route.Route_Route) *envoy_api_v2_route.Route_Route {
	r.Route.HashPolicy = append(r.Route.HashPolicy, &envoy_api_v2_route.RouteAction_HashPolicy{
		PolicySpecifier: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie_{
			Cookie: &envoy_api_v2_route.RouteAction_HashPolicy_Cookie{
				Name: "X-Contour-Session-Affinity",
				Ttl:  protobuf.Duration(0),
				Path: "/",
			},
		},
	})
	return r
}

func routecluster(cluster string) *envoy_api_v2_route.Route_Route {
	return &envoy_api_v2_route.Route_Route{
		Route: &envoy_api_v2_route.RouteAction{
			ClusterSpecifier: &envoy_api_v2_route.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}
}

func routeweightedcluster(clusters ...weightedcluster) *envoy_api_v2_route.Route_Route {
	return &envoy_api_v2_route.Route_Route{
		Route: &envoy_api_v2_route.RouteAction{
			ClusterSpecifier: &envoy_api_v2_route.RouteAction_WeightedClusters{
				WeightedClusters: weightedclusters(clusters),
			},
		},
	}
}

func weightedclusters(clusters []weightedcluster) *envoy_api_v2_route.WeightedCluster {
	var wc envoy_api_v2_route.WeightedCluster
	var total uint32
	for _, c := range clusters {
		total += c.weight
		wc.Clusters = append(wc.Clusters, &envoy_api_v2_route.WeightedCluster_ClusterWeight{
			Name:   c.name,
			Weight: protobuf.UInt32(c.weight),
		})
	}
	wc.TotalWeight = protobuf.UInt32(total)
	return &wc
}

func websocketroute(c string) *envoy_api_v2_route.Route_Route {
	cl := routecluster(c)
	cl.Route.UpgradeConfigs = append(cl.Route.UpgradeConfigs,
		&envoy_api_v2_route.RouteAction_UpgradeConfig{
			UpgradeType: "websocket",
		},
	)
	return cl
}

func prefixrewriteroute(c string) *envoy_api_v2_route.Route_Route {
	cl := routecluster(c)
	cl.Route.PrefixRewrite = "/"
	return cl
}

func clustertimeout(c string, timeout time.Duration) *envoy_api_v2_route.Route_Route {
	cl := routecluster(c)
	cl.Route.Timeout = protobuf.Duration(timeout)
	return cl
}

func service(ns, name string, ports ...v1.ServicePort) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v1.ServiceSpec{
			Ports: ports,
		},
	}
}

func externalnameservice(ns, name, externalname string, ports ...v1.ServicePort) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v1.ServiceSpec{
			Ports:        ports,
			ExternalName: externalname,
			Type:         v1.ServiceTypeExternalName,
		},
	}
}

func routeretry(cluster string, retryOn string, numRetries uint32, perTryTimeout time.Duration) *envoy_api_v2_route.Route_Route {
	r := routecluster(cluster)
	r.Route.RetryPolicy = &envoy_api_v2_route.RetryPolicy{
		RetryOn: retryOn,
	}
	if numRetries > 0 {
		r.Route.RetryPolicy.NumRetries = protobuf.UInt32(numRetries)
	}
	if perTryTimeout > 0 {
		r.Route.RetryPolicy.PerTryTimeout = protobuf.Duration(perTryTimeout)
	}
	return r
}
