// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"errors"
	"testing"

	"github.com/juju/tc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/juju/juju/core/network"
)

type k8sNetworkingSuite struct {
}

func TestK8sNetworkingSuite(t *testing.T) {
	tc.Run(t, &k8sNetworkingSuite{})
}

func (s *k8sNetworkingSuite) TestSupportsSpaces(c *tc.C) {
	// Arrange
	envNet := &environNetworking{}

	// Act
	ok, err := envNet.SupportsSpaces()

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsFalse)
}

func (s *k8sNetworkingSuite) TestSubnets(c *tc.C) {
	// Arrange
	envNet := &environNetworking{}

	// Act
	result, err := envNet.Subnets(c.Context(), nil)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []network.SubnetInfo{
		{
			CIDR: "0.0.0.0/0",
		}, {
			CIDR: "::/0",
		},
	})
}

func (s *k8sNetworkingSuite) TestSubnetsFromNodePodCIDRs(c *tc.C) {
	// Arrange
	envNet := newTestEnvironNetworking(&corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "a",
				},
				Spec: corev1.NodeSpec{
					PodCIDR: "10.10.0.0/24",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "b",
				},
				Spec: corev1.NodeSpec{
					PodCIDRs: []string{"fd10::/64", "10.10.1.0/24"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c",
				},
				Spec: corev1.NodeSpec{
					PodCIDRs: []string{"10.10.0.0/24", "not-a-cidr"},
				},
			},
		},
	})

	// Act
	result, err := envNet.Subnets(c.Context(), nil)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []network.SubnetInfo{
		{
			CIDR:       "10.10.0.0/24",
			ProviderId: "10.10.0.0/24",
		}, {
			CIDR:       "10.10.1.0/24",
			ProviderId: "10.10.1.0/24",
		}, {
			CIDR:       "fd10::/64",
			ProviderId: "fd10::/64",
		},
	})
}

func (s *k8sNetworkingSuite) TestSubnetsFromNodeCIDRAnnotations(c *tc.C) {
	// Arrange
	envNet := newTestEnvironNetworking(&corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "a",
					Annotations: map[string]string{
						"projectcalico.org/IPv4Address": "10.32.4.17/24",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "b",
					Annotations: map[string]string{
						"cilium.io/ipv6-pod-cidr": "fd32::/64",
					},
				},
			},
		},
	})

	// Act
	result, err := envNet.Subnets(c.Context(), nil)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []network.SubnetInfo{
		{
			CIDR:       "10.32.4.0/24",
			ProviderId: "10.32.4.0/24",
		}, {
			CIDR:       "fd32::/64",
			ProviderId: "fd32::/64",
		},
	})
}

func (s *k8sNetworkingSuite) TestSubnetsNodeDiscoveryErrorFallsBack(c *tc.C) {
	// Arrange
	clientset := fake.NewClientset()
	clientset.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, errors.New("boom")
	})
	envNet := &environNetworking{clientset: clientset}

	// Act
	result, err := envNet.Subnets(c.Context(), nil)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []network.SubnetInfo{
		{
			CIDR: "0.0.0.0/0",
		}, {
			CIDR: "::/0",
		},
	})
}

func newTestEnvironNetworking(objects ...k8sruntime.Object) *environNetworking {
	envNet := newEnvironNetworking(fake.NewClientset(objects...))
	return &envNet
}
