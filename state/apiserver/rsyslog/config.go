package rsyslog

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	apirsyslog "launchpad.net/juju-core/state/api/rsyslog"
)

// newRsyslogConfig creates a new instance of the RsyslogConfig.
func newRsyslogConfig(envCfg *config.Config, api *RsyslogAPI) (*apirsyslog.RsyslogConfig, error) {
	stateAddrsResult, err := api.StateAddresser.StateAddresses()
	if err != nil {
		return nil, err
	}
	port := envCfg.SyslogPort()

	apiAddresses := instance.NewAddresses(stateAddrsResult.Result...)

	return &apirsyslog.RsyslogConfig{
		CACert:    envCfg.RsyslogCACert(),
		Port:      port,
		HostPorts: instance.AddressesWithPort(apiAddresses, port),
	}, nil
}
