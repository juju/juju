// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log/syslog"
)

func confParams(context Context) (machineTag, namespace string, port int, err error) {
	namespace = context.AgentConfig().Value(agent.Namespace)
	machineTag = context.AgentConfig().Tag()

	environment := context.APIState().Environment()
	config, err := environment.EnvironConfig()
	if err != nil {
		return "", "", 0, err
	}
	port = config.SyslogPort()
	return machineTag, namespace, port, err
}

// upgradeStateServerRsyslogConfig upgrades a rsuslog config file on a state server.
func upgradeStateServerRsyslogConfig(context Context) (err error) {
	machineTag, namespace, syslogPort, err := confParams(context)
	configRenderer := syslog.NewAccumulateConfig(machineTag, syslogPort, namespace)
	data, err := configRenderer.Render()
	if err != nil {
		return nil
	}
	return WriteReplacementFile(environs.RsyslogConfPath, []byte(data))
}

// upgradeHostMachineRsyslogConfig upgrades a rsuslog config file on a host machine.
func upgradeHostMachineRsyslogConfig(context Context) (err error) {
	machineTag, namespace, syslogPort, err := confParams(context)
	addr, err := context.AgentConfig().APIAddresses()
	if err != nil {
		return err
	}

	configRenderer := syslog.NewForwardConfig(machineTag, syslogPort, namespace, addr)
	data, err := configRenderer.Render()
	if err != nil {
		return nil
	}
	return WriteReplacementFile(environs.RsyslogConfPath, []byte(data))
}
