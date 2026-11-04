// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap

import (
	"context"
	"net"
	"slices"

	"github.com/juju/juju/core/logger"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
)

const loopbackIP = "127.0.0.1"

// DqliteBindAddressResolverFunc selects an appropriate Dqlite
// bind address from the input list.
type DqliteBindAddressResolverFunc func(context.Context, corenetwork.ProviderAddresses) (string, error)

type dqliteBindAddressResolver struct {
	logger                logger.Logger
	lookupIP              func(context.Context, string, string) ([]net.IP, error)
	interfaceAddrs        func() ([]net.Addr, error)
	sourceAddressForRoute func(context.Context, net.IP) (net.IP, error)
}

type resolvedProviderAddress struct {
	value string
	ips   []net.IP
}

// NewDqliteBindAddressResolver returns a resolver backed by the host's DNS,
// interface and routing configuration.
func NewDqliteBindAddressResolver(logger logger.Logger) DqliteBindAddressResolverFunc {
	return dqliteBindAddressResolver{
		logger:                logger,
		lookupIP:              net.DefaultResolver.LookupIP,
		interfaceAddrs:        net.InterfaceAddrs,
		sourceAddressForRoute: sourceAddressForRoute,
	}.resolve
}

func (r dqliteBindAddressResolver) resolve(
	ctx context.Context, providerAddresses corenetwork.ProviderAddresses,
) (string, error) {
	if len(providerAddresses) == 0 {
		return "", errors.New("Dqlite bootstrap address not found")
	}

	resolved := make([]resolvedProviderAddress, 0, len(providerAddresses))
	var firstLookupError error
	for _, providerAddress := range providerAddresses {
		ips, err := r.resolveProviderAddress(ctx, providerAddress)
		if err != nil {
			r.logger.Debugf(ctx, "skipping potential Dqlite bind address address: %v", err)
			if firstLookupError == nil {
				firstLookupError = err
			}
			continue
		}

		for _, ip := range ips {
			if ip.IsLoopback() {
				return providerAddress.Value, nil
			}
		}

		resolved = append(resolved, resolvedProviderAddress{
			value: providerAddress.Value,
			ips:   ips,
		})
	}

	if len(resolved) == 0 {
		if firstLookupError != nil {
			return "", errors.Errorf("resolving potential Dqlite bind addresses: %w", firstLookupError)
		}
		return "", errors.New("suitable Dqlite bind address not found")
	}

	localAddresses, err := r.interfaceAddrs()
	if err != nil {
		return "", errors.Errorf("querying local network addresses: %w", err)
	}
	localIPs := interfaceIPs(localAddresses)
	for _, providerAddress := range resolved {
		if anyIPMatches(providerAddress.ips, localIPs) {
			// Keep the provider representation, particularly a Kubernetes
			// headless-Service FQDN, when it denotes this host.
			return providerAddress.value, nil
		}
	}

	// None of the provider identities is configured locally.
	// Ask the kernel which source address it would use to reach them.
	// This follows actual routing without inferring reachability from
	// Juju address scopes.
	for _, providerAddress := range resolved {
		for _, ip := range providerAddress.ips {
			sourceAddress, err := r.sourceAddressForRoute(ctx, ip)
			if err != nil {
				r.logger.Debugf(ctx, "skipping potential Dqlite bind address %q: %v", ip, err)
				continue
			}
			if sourceAddress == nil || sourceAddress.IsUnspecified() {
				r.logger.Debugf(
					ctx, "skipping potential Dqlite bind address %q: unusable source address %q", ip, sourceAddress)
				continue
			}
			return sourceAddress.String(), nil
		}
	}
	if err := ctx.Err(); err != nil {
		return "", errors.Capture(err)
	}

	// If no route exists, fall back to the loopback IP.
	// Entering HA will require the user to bind the controller charm's cluster
	// peer relation to a suitable space, which results in a singular Dqlite
	// bind address.
	r.logger.Warningf(ctx,
		"could not determine a local Dqlite bind address from provider addresses %v; falling back to %s",
		providerAddresses.Values(), loopbackIP)
	return loopbackIP, nil
}

func (r dqliteBindAddressResolver) resolveProviderAddress(
	ctx context.Context, address corenetwork.ProviderAddress,
) ([]net.IP, error) {
	if ip := address.IP(); ip != nil {
		return []net.IP{ip}, nil
	}
	ips, err := r.lookupIP(ctx, "ip", address.Value)
	if err != nil {
		return nil, errors.Errorf("resolving provider address %q: %w", address.Value, err)
	}
	if len(ips) == 0 {
		return nil, errors.Errorf("provider address %q resolved to no IPs", address.Value)
	}
	return ips, nil
}

func interfaceIPs(addresses []net.Addr) []net.IP {
	result := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		switch address := address.(type) {
		case *net.IPAddr:
			result = append(result, address.IP)
		case *net.IPNet:
			result = append(result, address.IP)
		}
	}
	return result
}

func anyIPMatches(candidates, local []net.IP) bool {
	for _, candidate := range candidates {
		if slices.ContainsFunc(local, candidate.Equal) {
			return true
		}
	}
	return false
}

func sourceAddressForRoute(ctx context.Context, remoteIP net.IP) (net.IP, error) {
	network := "udp6"
	if remoteIP.To4() != nil {
		network = "udp4"
	} else if remoteIP.To16() == nil {
		return nil, errors.Errorf("invalid remote IP %q", remoteIP)
	}

	// Connecting a UDP socket performs route selection without sending data.
	conn, err := (&net.Dialer{}).DialContext(
		ctx, network, net.JoinHostPort(remoteIP.String(), "9"),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	defer func() { _ = conn.Close() }()

	localAddress, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || localAddress.IP == nil {
		return nil, errors.Errorf("route returned invalid local address %q", conn.LocalAddr())
	}
	return localAddress.IP, nil
}
