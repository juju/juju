// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/service/common"
)

const (
	maxAgentFiles = 20000

	agentServiceTimeout = 300 // 5 minutes
)

// AgentConf returns the data that defines an init service config
// for the identified agent.
func AgentConf(info AgentInfo, renderer shell.Renderer) common.Conf {
	conf := common.Conf{
		Desc:          fmt.Sprintf("juju agent for %s", info.name),
		ExecStart:     info.cmd(renderer),
		Logfile:       info.logFile(renderer),
		Env:           osenv.FeatureFlags(),
		Timeout:       agentServiceTimeout,
		ServiceBinary: info.jujud(renderer),
		ServiceArgs:   info.execArgs(renderer),
	}

	switch info.Kind {
	case AgentKindMachine:
		conf.Limit = map[string]int{
			"nofile": maxAgentFiles,
		}
	case AgentKindUnit:
		conf.Desc = "juju unit agent for " + info.ID
	}

	return conf
}

// TODO(ericsnow) Eliminate ContainerAgentConf once it is no longer
// used in worker/deployer/simple.go.

// ContainerAgentConf returns the data that defines an init service config
// for the identified agent running in a container.
func ContainerAgentConf(info AgentInfo, renderer shell.Renderer, containerType string) common.Conf {
	conf := AgentConf(info, renderer)

	// TODO(thumper): 2013-09-02 bug 1219630
	// As much as I'd like to remove JujuContainerType now, it is still
	// needed as MAAS still needs it at this stage, and we can't fix
	// everything at once.
	envVars := map[string]string{
		osenv.JujuContainerTypeEnvKey: containerType,
	}
	osenv.MergeEnvironment(envVars, conf.Env)
	conf.Env = envVars

	return conf
}

// ShutdownAfterConf builds a service conf that will cause the host to
// shut down after the named service stops.
func ShutdownAfterConf(serviceName string) (common.Conf, error) {
	if serviceName == "" {
		return common.Conf{}, errors.New(`missing "after" service name`)
	}
	desc := "juju shutdown job"
	return shutdownAfterConf(serviceName, desc), nil
}

func shutdownAfterConf(serviceName, desc string) common.Conf {
	return common.Conf{
		Desc:         desc,
		Transient:    true,
		AfterStopped: serviceName,
		ExecStart:    "/sbin/shutdown -h now",
	}
}
