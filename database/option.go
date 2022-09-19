package database

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"

	"github.com/canonical/go-dqlite/app"
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/network"
)

const (
	dqliteDataDir = "dqlite"
	dqlitePort    = 17666
)

// OptionFactory creates Dqlite `App` initialisation arguments and options.
// These generally depend on a controller's agent config.
type OptionFactory struct {
	cfg            agent.Config
	port           int
	interfaceAddrs func() ([]net.Addr, error)
}

// NewOptionFactory returns a new OptionFactory reference
// based on the input agent configuration.
func NewOptionFactory(cfg agent.Config) *OptionFactory {
	return &OptionFactory{
		cfg:            cfg,
		port:           dqlitePort,
		interfaceAddrs: net.InterfaceAddrs,
	}
}

// EnsureDataDir ensures that a directory for Dqlite data exists at
// a path determined by the agent config, then returns that path.
func (f *OptionFactory) EnsureDataDir() (string, error) {
	dir := filepath.Join(f.cfg.DataDir(), dqliteDataDir)
	err := os.MkdirAll(dir, 0700)
	return dir, errors.Annotatef(err, "creating directory for Dqlite data")
}

// WithAddressOption returns a Dqlite application Option
// for specifying the local address:port to use.
// TODO (manadart 2022-09-07): We will need to consider what happens with
// juju-ha-space controller configuration as it relates to this address.
// We should also look at taking the config as a config setter,
// writing the address after the first determination, then just reading it
// thereafter so that it never changes.
func (f *OptionFactory) WithAddressOption() (app.Option, error) {
	sysAddrs, err := f.interfaceAddrs()
	if err != nil {
		return nil, errors.Annotate(err, "querying addresses of system NICs")
	}

	var addrs network.MachineAddresses
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

		addrs = append(addrs, network.NewMachineAddress(host))
	}

	cloudLocal := addrs.AllMatchingScope(network.ScopeMatchCloudLocal)
	if len(cloudLocal) == 0 {
		return nil, errors.NewNotFound(nil, "no suitable local address for advertising to Dqlite peers")
	}

	// Sort to ensure that the same address is returned for multi-nic/address
	// systems. Dqlite does not allow it to change between node restarts.
	values := cloudLocal.Values()
	sort.Strings(values)
	return app.WithAddress(fmt.Sprintf("%s:%d", values[0], f.port)), nil
}
