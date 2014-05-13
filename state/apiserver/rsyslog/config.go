package rsyslog

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
)

type RsyslogConfig struct {
	*config.Config

	CACert    string
	Port      int
	HostPorts []instance.HostPort
}

func NewRsyslogConfig(envCfg *config.Config, st *state.State) (*RsyslogConfig, error) {
	apiAddresses, err := st.APIAddresses()
	if err != nil {
		return nil, err
	}
	port := envCfg.SyslogPort()
	return &RsyslogConfig{
		CACert:    envCfg.RsyslogCACert(),
		Port:      port,
		HostPorts: instance.AddressesWithPort(apiAddresses, port),
	}, nil
}
