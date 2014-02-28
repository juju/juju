// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log/syslog"
)

func confParams(context Context) (machineTag, logDir, namespace string, port int, err error) {
	namespace = context.AgentConfig().Value(agent.Namespace)
	machineTag = context.AgentConfig().Tag()
	logDir = context.AgentConfig().LogDir()

	environment := context.APIState().Environment()
	config, err := environment.EnvironConfig()
	if err != nil {
		return "", "", "", 0, err
	}
	port = config.SyslogPort()
	return machineTag, logDir, namespace, port, err
}

// upgradeStateServerRsyslogConfig upgrades a rsyslog config file on a state server.
func upgradeStateServerRsyslogConfig(context Context) (err error) {
	machineTag, logDir, namespace, syslogPort, err := confParams(context)
	configRenderer := syslog.NewAccumulateConfig(machineTag, logDir, syslogPort, namespace)
	data, err := configRenderer.Render()
	if err != nil {
		return nil
	}
	return WriteReplacementFile(environs.RsyslogConfPath, []byte(data), 0644)
}

// upgradeHostMachineRsyslogConfig upgrades a rsyslog config file on a host machine.
func upgradeHostMachineRsyslogConfig(context Context) (err error) {
	machineTag, logDir, namespace, syslogPort, err := confParams(context)
	addr, err := context.AgentConfig().APIAddresses()
	if err != nil {
		return err
	}

	configRenderer := syslog.NewForwardConfig(machineTag, logDir, syslogPort, namespace, addr)
	data, err := configRenderer.Render()
	if err != nil {
		return nil
	}
	return WriteReplacementFile(environs.RsyslogConfPath, []byte(data), 0644)
}
