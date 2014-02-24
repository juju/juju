// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"path/filepath"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log/syslog"
)

type rsyslogConfParams struct {
	machineTag string
	namespace  string
	port       int
	tls        bool
}

func getRsyslogConfParams(context Context) (*rsyslogConfParams, error) {
	environment := context.APIState().Environment()
	config, err := environment.EnvironConfig()
	if err != nil {
		return nil, err
	}
	return &rsyslogConfParams{
		namespace:  context.AgentConfig().Value(agent.Namespace),
		machineTag: context.AgentConfig().Tag(),
		port:       config.SyslogPort(),
		tls:        config.SyslogTLS(),
	}, nil
}

// upgradeStateServerRsyslogConfig upgrades a rsyslog config file on a state server.
func upgradeStateServerRsyslogConfig(context Context) (err error) {
	params, err := getRsyslogConfParams(context)
	if err != nil {
		return err
	}
	configRenderer := syslog.NewAccumulateConfig(params.machineTag, params.port, params.namespace)
	// TODO(axw) 2014-02-24 #1284020
	// Override the configRenderer.LogDir default.
	if params.tls {
		configRenderer.TLSCACertPath = filepath.Join(configRenderer.LogDir, "ca.pem")
		configRenderer.TLSCertPath = filepath.Join(configRenderer.LogDir, "cert.pem")
		configRenderer.TLSKeyPath = filepath.Join(configRenderer.LogDir, "key.pem")
	}
	data, err := configRenderer.Render()
	if err != nil {
		return nil
	}
	return WriteReplacementFile(environs.RsyslogConfPath, []byte(data), 0644)
}

// upgradeHostMachineRsyslogConfig upgrades a rsyslog config file on a host machine.
func upgradeHostMachineRsyslogConfig(context Context) (err error) {
	params, err := getRsyslogConfParams(context)
	if err != nil {
		return err
	}
	addr, err := context.AgentConfig().APIAddresses()
	if err != nil {
		return err
	}
	configRenderer := syslog.NewForwardConfig(params.machineTag, params.port, params.namespace, addr)
	// TODO(axw) 2014-02-24 #1284020
	// Override the configRenderer.LogDir default.
	if params.tls {
		configRenderer.TLSCACertPath = filepath.Join(configRenderer.LogDir, "ca.pem")
	}
	data, err := configRenderer.Render()
	if err != nil {
		return nil
	}
	return WriteReplacementFile(environs.RsyslogConfPath, []byte(data), 0644)
}
