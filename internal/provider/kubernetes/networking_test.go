// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"errors"
	"testing"

	"github.com/juju/tc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	envNet := &environNetworking{
		listNodes: func(context.Context) ([]corev1.Node, error) {
			return []corev1.Node{
				{
					Spec: corev1.NodeSpec{
						PodCIDR: "10.10.0.0/24",
					},
				},
				{
					Spec: corev1.NodeSpec{
						PodCIDRs: []string{"fd10::/64", "10.10.1.0/24"},
					},
				},
				{
					Spec: corev1.NodeSpec{
						PodCIDRs: []string{"10.10.0.0/24", "not-a-cidr"},
					},
				},
			}, nil
		},
	}

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
	envNet := &environNetworking{
		listNodes: func(context.Context) ([]corev1.Node, error) {
			return []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"projectcalico.org/IPv4Address": "10.32.4.17/24",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"cilium.io/ipv6-pod-cidr": "fd32::/64",
						},
					},
				},
			}, nil
		},
	}

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
	envNet := &environNetworking{
		listNodes: func(context.Context) ([]corev1.Node, error) {
			return nil, errors.New("boom")
		},
	}

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
