package rsyslog

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api/common"
)

type RsyslogConfig struct {
	*config.Config

	CACert string
	Port   int
}

func (cfg *RsyslogConfig) ReadFromEnvironConfig(environConfig *config.Config) {
	cfg.CACert = environConfig.RsyslogCACert()
	cfg.Port = environConfig.SyslogPort()
}

func (cfg *RsyslogConfig) HostPorts(st State) [][]instance.HostPort {
	hostPorts := st.APIHostPorts()
	return hostPorts
}
