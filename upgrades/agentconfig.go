// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"
	"os/user"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

func migrateLocalProviderEnvSettings(st *state.State, envConfig *config.Config, namespace, container *string) error {
	return nil
}

func migrateLocalProviderAgentConfig(context Context, target Target) error {
	st := context.State()
	envConfig, err := st.EnvironConfig()
	if err != nil {
		return fmt.Errorf("failed to read current config: %v", err)
	}
	if envConfig.Type() != "local" {
		return nil
	}
	attrs := envConfig.AllAttrs()
	rootDir, _ := attrs["root-dir"].(string)
	// In case these two are empty we need to set them and update the
	// environment config.
	namespace, _ := attrs["namespace"].(string)
	container, _ := attrs["container"].(string)

	if namespace == "" {
		curUser, err := user.Current()
		if err != nil {
			return fmt.Errorf("cannot get current user: %v", err)
		}
		namespace = curUser.Username + "-" + envConfig.Name()
	}
	if container == "" {
		container = "lxc"
	}

	newCfg, err := envConfig.Apply(map[string]interface{}{
		"namespace": namespace,
		"container": container,
	})
	if err != nil {
		return fmt.Errorf("cannot apply environment config: %v", err)
	}
	err = st.SetEnvironConfig(newCfg, envConfig)
	if err != nil {
		return fmt.Errorf("cannot set environment config: %v", err)
	}

	// These three change according to target.
	dataDir := agent.DefaultDataDir
	logDir := agent.DefaultLogDir
	jobs := []params.MachineJob{params.JobHostUnits}
	if target == StateServer {
		dataDir = rootDir
		// TODO(dimitern) create this (as root), so that rsyslog
		// can create its certificates and logs in it.
		logDir = fmt.Sprintf("/var/log/juju-%s", namespace)
		jobs = []params.MachineJob{params.JobManageEnviron}
	}

	migrateParams := agent.AgentConfigParams{
		DataDir: dataDir,
		LogDir:  logDir,
		Jobs:    jobs,
		Values: map[string]string{
			// Add new values from the environment.
			agent.Namespace:        namespace,
			agent.AgentServiceName: "juju-agent-" + namespace,
			agent.MongoServiceName: "juju-db-" + namespace,
			// Delete the obsolete values.
			"SHARED_STORAGE_ADDR": "",
			"SHARED_STORAGE_DIR":  "",
		},
	}
	return agent.MigrateConfig(context.AgentConfig(), migrateParams)
}
