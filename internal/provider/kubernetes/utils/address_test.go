// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

type addressSuite struct{}

func TestAddressSuite(t *stdtesting.T) {
	tc.Run(t, &addressSuite{})
}

func (s *addressSuite) TestGetSvcAddressesControllerServiceTypes(c *tc.C) {
	for _, test := range []struct {
		name             string
		service          *core.Service
		includeClusterIP bool
		expected         []network.ProviderAddress
	}{
		{
			name: "cluster ip",
			service: &core.Service{Spec: core.ServiceSpec{
				Type:      core.ServiceTypeClusterIP,
				ClusterIP: "10.152.183.205",
			}},
			expected: []network.ProviderAddress{
				network.NewMachineAddress("10.152.183.205", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
			},
		},
		{
			name: "load balancer",
			service: &core.Service{Spec: core.ServiceSpec{
				Type:           core.ServiceTypeLoadBalancer,
				ClusterIP:      "10.152.183.205",
				LoadBalancerIP: "203.0.113.10",
			}},
			expected: []network.ProviderAddress{
				network.NewMachineAddress("203.0.113.10", network.WithScope(network.ScopePublic)).AsProviderAddress(),
			},
		},
		{
			name: "load balancer with cluster ip",
			service: &core.Service{Spec: core.ServiceSpec{
				Type:           core.ServiceTypeLoadBalancer,
				ClusterIP:      "10.152.183.205",
				LoadBalancerIP: "203.0.113.10",
			}},
			includeClusterIP: true,
			expected: []network.ProviderAddress{
				network.NewMachineAddress("203.0.113.10", network.WithScope(network.ScopePublic)).AsProviderAddress(),
				network.NewMachineAddress("10.152.183.205", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
			},
		},
		{
			name: "external name with explicit ip",
			service: &core.Service{Spec: core.ServiceSpec{
				Type:         core.ServiceTypeExternalName,
				ExternalName: "controller.example.com",
				ExternalIPs:  []string{"203.0.113.10"},
			}},
			expected: []network.ProviderAddress{
				network.NewMachineAddress("203.0.113.10", network.WithScope(network.ScopePublic)).AsProviderAddress(),
				network.NewMachineAddress("controller.example.com", network.WithScope(network.ScopePublic)).AsProviderAddress(),
			},
		},
	} {
		c.Logf("testing %q", test.name)
		addrs := utils.GetSvcAddresses(test.service, test.includeClusterIP)
		c.Check(addrs, tc.DeepEquals, test.expected)
	}
}
