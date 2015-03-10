package rsyslog

import (
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

	apiHostPorts, err := network.ParseHostPorts(stateAddrsResult.Result...)
	if err != nil {
		return nil, err
	}
	apiAddresses := network.HostsWithoutPort(apiHostPorts)

	return &apirsyslog.RsyslogConfig{
		CACert:    envCfg.RsyslogCACert(),
		CAKey:     envCfg.RsyslogCAKey(),
		Port:      port,
		HostPorts: network.AddressesWithPort(apiAddresses, port),
	}, nil
}
