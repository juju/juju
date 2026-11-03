// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	coretesting "github.com/juju/juju/internal/testing"
)

type bindAddressSuite struct {
	coretesting.BaseSuite
}

func TestBindAddressSuite(t *testing.T) {
	tc.Run(t, &bindAddressSuite{})
}

func (s *bindAddressSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (*bindAddressSuite) TestPreservesLocallyConfiguredProviderIP(c *tc.C) {
	resolver := dqliteBindAddressResolver{
		lookupIP: unexpectedLookup(c),
		interfaceAddrs: func() ([]net.Addr, error) {
			return localAddresses("10.0.0.9"), nil
		},
		sourceAddressForRoute: unexpectedRoute(c),
	}

	address, err := resolver.resolve(c.Context(), providerAddresses("10.0.0.9"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(address, tc.Equals, "10.0.0.9")
}

func (*bindAddressSuite) TestPreservesEquivalentLocalIPv6(c *tc.C) {
	resolver := dqliteBindAddressResolver{
		lookupIP: unexpectedLookup(c),
		interfaceAddrs: func() ([]net.Addr, error) {
			return localAddresses("2001:db8::1"), nil
		},
		sourceAddressForRoute: unexpectedRoute(c),
	}

	address, err := resolver.resolve(
		c.Context(), providerAddresses("2001:db8:0:0:0:0:0:1"),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(address, tc.Equals, "2001:db8:0:0:0:0:0:1")
}

func (*bindAddressSuite) TestPreservesK8sHeadlessServiceName(c *tc.C) {
	const fqdn = "controller-0.controller-service-endpoints.controller-a.svc.cluster.local"
	resolver := dqliteBindAddressResolver{
		lookupIP: func(_ context.Context, network, host string) ([]net.IP, error) {
			c.Check(network, tc.Equals, "ip")
			c.Check(host, tc.Equals, fqdn)
			return []net.IP{net.ParseIP("10.1.2.3")}, nil
		},
		interfaceAddrs: func() ([]net.Addr, error) {
			return localAddresses("10.1.2.3"), nil
		},
		sourceAddressForRoute: unexpectedRoute(c),
	}

	address, err := resolver.resolve(c.Context(), providerAddresses(fqdn))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(address, tc.Equals, fqdn)
}

func (*bindAddressSuite) TestPrefersLocalProviderIdentityOverEarlierRemote(c *tc.C) {
	resolver := dqliteBindAddressResolver{
		lookupIP: unexpectedLookup(c),
		interfaceAddrs: func() ([]net.Addr, error) {
			return localAddresses("10.0.0.9"), nil
		},
		sourceAddressForRoute: unexpectedRoute(c),
	}

	address, err := resolver.resolve(
		c.Context(), providerAddresses("3.84.237.4", "10.0.0.9"),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(address, tc.Equals, "10.0.0.9")
}

func (*bindAddressSuite) TestNATIPAddressUsesRouteSource(c *tc.C) {
	resolver := dqliteBindAddressResolver{
		lookupIP: unexpectedLookup(c),
		interfaceAddrs: func() ([]net.Addr, error) {
			// This unrelated private address must not win merely because of
			// its inferred Juju scope.
			return localAddresses("10.44.0.1", "192.168.1.20"), nil
		},
		sourceAddressForRoute: func(_ context.Context, remote net.IP) (net.IP, error) {
			c.Check(remote.Equal(net.ParseIP("3.84.237.4")), tc.IsTrue)
			return net.ParseIP("192.168.1.20"), nil
		},
	}

	address, err := resolver.resolve(c.Context(), providerAddresses("3.84.237.4"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(address, tc.Equals, "192.168.1.20")
}

func (*bindAddressSuite) TestNATHostnameUsesRouteSource(c *tc.C) {
	const hostname = "controller.example.com"
	resolver := dqliteBindAddressResolver{
		lookupIP: func(_ context.Context, _, host string) ([]net.IP, error) {
			c.Check(host, tc.Equals, hostname)
			return []net.IP{net.ParseIP("3.84.237.4")}, nil
		},
		interfaceAddrs: func() ([]net.Addr, error) {
			return localAddresses("192.168.1.20"), nil
		},
		sourceAddressForRoute: func(_ context.Context, remote net.IP) (net.IP, error) {
			c.Check(remote.Equal(net.ParseIP("3.84.237.4")), tc.IsTrue)
			return net.ParseIP("192.168.1.20"), nil
		},
	}

	address, err := resolver.resolve(c.Context(), providerAddresses(hostname))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(address, tc.Equals, "192.168.1.20")
}

func (*bindAddressSuite) TestLoopbackProviderDoesNotQueryInterfaces(c *tc.C) {
	resolver := dqliteBindAddressResolver{
		lookupIP:              unexpectedLookup(c),
		interfaceAddrs:        unexpectedInterfaceAddrs(c),
		sourceAddressForRoute: unexpectedRoute(c),
	}

	address, err := resolver.resolve(c.Context(), providerAddresses("127.0.0.1"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(address, tc.Equals, "127.0.0.1")
}

func (*bindAddressSuite) TestNoRouteFallsBackToLoopback(c *tc.C) {
	resolver := dqliteBindAddressResolver{
		lookupIP: unexpectedLookup(c),
		interfaceAddrs: func() ([]net.Addr, error) {
			return localAddresses("10.0.0.9"), nil
		},
		sourceAddressForRoute: func(context.Context, net.IP) (net.IP, error) {
			return nil, errors.New("no route")
		},
	}

	address, err := resolver.resolve(c.Context(), providerAddresses("3.84.237.4"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(address, tc.Equals, loopbackIP)
}

func (*bindAddressSuite) TestNoProviderAddress(c *tc.C) {
	resolver := dqliteBindAddressResolver{}

	_, err := resolver.resolve(c.Context(), nil)
	c.Check(err, tc.ErrorMatches, "Dqlite bootstrap address not found")
}

func (*bindAddressSuite) TestHostnameResolutionError(c *tc.C) {
	expected := errors.New("lookup failed")
	resolver := dqliteBindAddressResolver{
		lookupIP: func(context.Context, string, string) ([]net.IP, error) {
			return nil, expected
		},
		interfaceAddrs:        unexpectedInterfaceAddrs(c),
		sourceAddressForRoute: unexpectedRoute(c),
	}

	_, err := resolver.resolve(c.Context(), providerAddresses("controller.example.com"))
	c.Check(err, tc.ErrorIs, expected)
}

func (*bindAddressSuite) TestInterfaceDiscoveryError(c *tc.C) {
	expected := errors.New("interfaces failed")
	resolver := dqliteBindAddressResolver{
		lookupIP: unexpectedLookup(c),
		interfaceAddrs: func() ([]net.Addr, error) {
			return nil, expected
		},
		sourceAddressForRoute: unexpectedRoute(c),
	}

	_, err := resolver.resolve(c.Context(), providerAddresses("3.84.237.4"))
	c.Check(err, tc.ErrorIs, expected)
}

func (*bindAddressSuite) TestRouteCancellationIsReturned(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	resolver := dqliteBindAddressResolver{
		lookupIP: unexpectedLookup(c),
		interfaceAddrs: func() ([]net.Addr, error) {
			return localAddresses("10.0.0.9"), nil
		},
		sourceAddressForRoute: func(context.Context, net.IP) (net.IP, error) {
			cancel()
			return nil, context.Canceled
		},
	}

	_, err := resolver.resolve(ctx, providerAddresses("3.84.237.4"))
	c.Check(err, tc.ErrorIs, context.Canceled)
}

func providerAddresses(values ...string) network.ProviderAddresses {
	return network.NewMachineAddresses(values).AsProviderAddresses()
}

func localAddresses(values ...string) []net.Addr {
	result := make([]net.Addr, len(values))
	for i, value := range values {
		ip := net.ParseIP(value)
		bits := 128
		if ip.To4() != nil {
			bits = 32
		}
		result[i] = &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
	}
	return result
}

func unexpectedLookup(c *tc.C) func(context.Context, string, string) ([]net.IP, error) {
	return func(context.Context, string, string) ([]net.IP, error) {
		c.Fatal("unexpected DNS lookup")
		return nil, nil
	}
}

func unexpectedInterfaceAddrs(c *tc.C) func() ([]net.Addr, error) {
	return func() ([]net.Addr, error) {
		c.Fatal("unexpected interface address query")
		return nil, nil
	}
}

func unexpectedRoute(c *tc.C) func(context.Context, net.IP) (net.IP, error) {
	return func(context.Context, net.IP) (net.IP, error) {
		c.Fatal("unexpected route query")
		return nil, nil
	}
}
