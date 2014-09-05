package rsyslog

import (
	"net"

	apirsyslog "github.com/juju/juju/api/rsyslog"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
)

// newRsyslogConfig creates a new instance of the RsyslogConfig.
func newRsyslogConfig(envCfg *config.Config, api *RsyslogAPI) (*apirsyslog.RsyslogConfig, error) {
	stateAddrsResult, err := api.StateAddresser.StateAddresses()
	if err != nil {
		return nil, err
	}
	port := envCfg.SyslogPort()

	var bareAddrs []string
	for _, addr := range stateAddrsResult.Result {
		hostOnly, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		bareAddrs = append(bareAddrs, hostOnly)
	}
	apiAddresses := network.NewAddresses(bareAddrs...)

	return &apirsyslog.RsyslogConfig{
		CACert:    envCfg.RsyslogCACert(),
		Port:      port,
		HostPorts: network.AddressesWithPort(apiAddresses, port),
	}, nil
}
