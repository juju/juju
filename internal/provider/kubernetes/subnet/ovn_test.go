// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"testing"

	"github.com/juju/tc"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type ovnSuite struct{}

func TestOVN(t *testing.T) {
	tc.Run(t, &ovnSuite{})
}

func (s *ovnSuite) TestOVNNodeSubnetsSingleStackString(c *tc.C) {
	clients := newClusterBuilder().
		addCore(node("a", nil, map[string]string{
			"k8s.ovn.org/node-subnets": `{"default":"10.130.0.0/23"}`,
		})).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.130.0.0/23"))
}

func (s *ovnSuite) TestOVNNodeSubnetsDualStackArray(c *tc.C) {
	clients := newClusterBuilder().
		addCore(node("a", nil, map[string]string{
			"k8s.ovn.org/node-subnets": `{"default":["10.130.0.0/23","fd01:0:0:2::/64"]}`,
		})).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.130.0.0/23", "fd01:0:0:2::/64"))
}

func (s *ovnSuite) TestOVNNodeSubnetsExcludesNonDefault(c *tc.C) {
	clients := newClusterBuilder().
		addCore(node("a", nil, map[string]string{
			"k8s.ovn.org/node-subnets":               `{"default":"10.130.0.0/23","secondary":"10.140.0.0/23"}`,
			"k8s.ovn.org/hybrid-overlay-node-subnet": "10.150.0.0/23",
		})).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.130.0.0/23"))
}

func (s *ovnSuite) TestOVNConfigNetCIDRHostPrefixStripped(c *tc.C) {
	clients := newClusterBuilder().
		addCore(configMap("ovn-kubernetes", "ovn-config", map[string]string{
			"net_cidr": "10.128.0.0/14/23,fd01::/48/64",
		})).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.128.0.0/14", "fd01::/48"))
}

func (s *ovnSuite) TestOVNConfigNetCIDROpenShiftNamespace(c *tc.C) {
	clients := newClusterBuilder().
		addCore(configMap("openshift-ovn-kubernetes", "ovn-config", map[string]string{
			"net_cidr": "10.128.0.0/14/23",
		})).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.128.0.0/14"))
}

func (s *ovnSuite) TestOVNOpenShiftNetworkCRD(c *tc.C) {
	netObj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "config.openshift.io/v1",
		"kind":       "Network",
		"metadata":   map[string]any{"name": "cluster"},
		"status": map[string]any{
			"clusterNetwork": []any{
				map[string]any{"cidr": "10.128.0.0/14", "hostPrefix": int64(23)},
			},
		},
	}}
	clients := newClusterBuilder().
		addCRD("networks", "config.openshift.io", "v1").
		addCR(netObj).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.128.0.0/14"))
}

func (s *ovnSuite) TestOVNUnionOfSources(c *tc.C) {
	netObj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "config.openshift.io/v1",
		"kind":       "Network",
		"metadata":   map[string]any{"name": "cluster"},
		"status": map[string]any{
			"clusterNetwork": []any{map[string]any{"cidr": "10.128.0.0/14"}},
		},
	}}
	clients := newClusterBuilder().
		addCore(
			node("a", nil, map[string]string{"k8s.ovn.org/node-subnets": `{"default":"10.130.0.0/23"}`}),
			configMap("ovn-kubernetes", "ovn-config", map[string]string{"net_cidr": "10.128.0.0/14/23"}),
		).
		addCRD("networks", "config.openshift.io", "v1").
		addCR(netObj).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.128.0.0/14", "10.130.0.0/23"))
}
