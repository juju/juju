package database

import (
	"fmt"
	"net"

	"github.com/canonical/go-dqlite/app"
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/network"
)

const dqlitePort = 17666

// OptionFactory creates Dqlite `App` initialisation options.
// These generally depend on a controller's agent config.
type OptionFactory struct {
	cfg            agent.Config
	port           int
	interfaceAddrs func() ([]net.Addr, error)
}

// NewOptionFactoryWithDefaults returns a new OptionFactory reference
// based on the input config, but otherwise using defaults.
func NewOptionFactoryWithDefaults(cfg agent.Config) *OptionFactory {
	return NewOptionFactory(cfg, dqlitePort, net.InterfaceAddrs)
}

// NewOptionFactory returns a new OptionFactory reference
// based on the input arguments.
func NewOptionFactory(cfg agent.Config, port int, interfaceAddrs func() ([]net.Addr, error)) *OptionFactory {
	return &OptionFactory{
		cfg:            cfg,
		port:           port,
		interfaceAddrs: interfaceAddrs,
	}
}

// WithAddressOption returns a Dqlite application option
// for specifying the local address:port to use.
// TODO (manadart 2022-09-07): We will need to consider what happens with
// juju-ha-space controller configuration as it relates to this address.
// Consider also using the generic address collection functions to filter.
func (f *OptionFactory) WithAddressOption() (app.Option, error) {
	sysAddrs, err := f.interfaceAddrs()
	if err != nil {
		return nil, errors.Annotate(err, "querying addresses of system NICs")
	}

	// Dqlite nodes should only advertise cloud-local addresses to their
	// peers. To figure out the address to bind to, we need to scan all NIC
	// addresses and return the first one that has cloud-local scope.
	for _, sysAddr := range sysAddrs {
		var host string

		switch v := sysAddr.(type) {
		case *net.IPNet:
			host = v.IP.String()
		case *net.IPAddr:
			host = v.IP.String()
		default:
			continue
		}

		machAddr := network.NewMachineAddress(host)
		if machAddr.Scope == network.ScopeCloudLocal {
			return app.WithAddress(fmt.Sprintf("%s:%d", machAddr.Value, f.port)), nil
		}
	}

	return nil, errors.NewNotFound(nil, "no suitable local address for advertising to Dqlite peers")
}
