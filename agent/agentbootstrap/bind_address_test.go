// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	loggertesting "github.com/juju/juju/internal/logger/testing"
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

func (*bindAddressSuite) TestNewDqliteBindAddressResolver(c *tc.C) {
	resolve := NewDqliteBindAddressResolver(loggertesting.WrapCheckLog(c))
	for _, providerAddress := range []string{"127.0.0.1", "::1", "localhost"} {
		address, err := resolve(c.Context(), providerAddresses(providerAddress))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(address, tc.Equals, providerAddress)
	}
}

func (*bindAddressSuite) TestPreservesLocallyConfiguredProviderIP(c *tc.C) {
	resolver := dqliteBindAddressResolver{
		logger:   loggertesting.WrapCheckLog(c),
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
		logger:   loggertesting.WrapCheckLog(c),
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
		logger: loggertesting.WrapCheckLog(c),
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
		logger:   loggertesting.WrapCheckLog(c),
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
		logger:   loggertesting.WrapCheckLog(c),
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
		logger: loggertesting.WrapCheckLog(c),
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
		logger:                loggertesting.WrapCheckLog(c),
		lookupIP:              unexpectedLookup(c),
		interfaceAddrs:        unexpectedInterfaceAddrs(c),
		sourceAddressForRoute: unexpectedRoute(c),
	}

	address, err := resolver.resolve(c.Context(), providerAddresses("127.0.0.1"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(address, tc.Equals, "127.0.0.1")
}

func (*bindAddressSuite) TestLoopbackProviderTakesPrecedenceOverLocalProvider(c *tc.C) {
	resolver := dqliteBindAddressResolver{
		logger: loggertesting.WrapCheckLog(c),
		lookupIP: func(_ context.Context, _, host string) ([]net.IP, error) {
			c.Check(host, tc.Equals, "localhost")
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		},
		interfaceAddrs: func() ([]net.Addr, error) {
			return localAddresses("10.0.0.9", "127.0.0.1", "::1"), nil
		},
		sourceAddressForRoute: unexpectedRoute(c),
	}

	for _, loopback := range []string{"127.0.0.1", "::1", "localhost"} {
		for _, addresses := range []network.ProviderAddresses{
			providerAddresses("10.0.0.9", loopback),
			providerAddresses(loopback, "10.0.0.9"),
		} {
			address, err := resolver.resolve(c.Context(), addresses)
			c.Assert(err, tc.ErrorIsNil)
			c.Check(address, tc.Equals, loopback,
				tc.Commentf("provider addresses: %v", addresses.Values()))
		}
	}
}

func (*bindAddressSuite) TestNoRouteFallsBackToLoopback(c *tc.C) {
	resolver := dqliteBindAddressResolver{
		logger:   loggertesting.WrapCheckLog(c),
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
		logger: loggertesting.WrapCheckLog(c),
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
		logger:   loggertesting.WrapCheckLog(c),
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
		logger:   loggertesting.WrapCheckLog(c),
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

func (*bindAddressSuite) TestSourceAddressForRouteIPv4(c *tc.C) {
	remote := net.ParseIP("127.0.0.1")
	source, err := sourceAddressForRoute(c.Context(), remote)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(source.Equal(remote), tc.IsTrue)
}

func (*bindAddressSuite) TestSourceAddressForRouteIPv6(c *tc.C) {
	// Check IPv6 availability independently of the route probe under test.
	listener, err := net.ListenPacket("udp6", "[::1]:0")
	if errors.Is(err, syscall.EAFNOSUPPORT) ||
		errors.Is(err, syscall.EPROTONOSUPPORT) ||
		errors.Is(err, syscall.EADDRNOTAVAIL) {
		c.Skip("IPv6 loopback is unavailable")
	}
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { _ = listener.Close() })

	remote := net.ParseIP("::1")
	source, err := sourceAddressForRoute(c.Context(), remote)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(source.Equal(remote), tc.IsTrue)
}

func (*bindAddressSuite) TestSourceAddressForRouteInvalidIP(c *tc.C) {
	for _, remote := range []net.IP{nil, {1, 2, 3}} {
		source, err := sourceAddressForRoute(c.Context(), remote)
		c.Check(err, tc.ErrorMatches, "invalid remote IP .*")
		c.Check(source, tc.IsNil)
	}
}

func (*bindAddressSuite) TestSourceAddressForRouteCancelled(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	cancel()
	for _, remote := range []string{"127.0.0.1", "::1"} {
		source, err := sourceAddressForRoute(ctx, net.ParseIP(remote))
		c.Check(err, tc.ErrorIs, context.Canceled)
		c.Check(source, tc.IsNil)
	}
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
