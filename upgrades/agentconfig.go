// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"
	"os"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state/api/params"
)

var rootLogDir = "/var/log"

func migrateLocalProviderAgentConfig(context Context) error {
	st := context.State()
	if st == nil {
		// We're running on a different node than the state server.
		return nil
	}
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
		username := os.Getenv("USER")
		if username == "root" {
			// sudo was probably called, get the original user.
			username = os.Getenv("SUDO_USER")
		}
		if username == "" {
			return fmt.Errorf("cannot get current user from the environment: %v", os.Environ())
		}
		namespace = username + "-" + envConfig.Name()
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
	if err := st.SetEnvironConfig(newCfg, envConfig); err != nil {
		return fmt.Errorf("cannot set environment config: %v", err)
	}

	dataDir := rootDir
	// rsyslogd is restricted to write to /var/log
	logDir := fmt.Sprintf("%s/juju-%s", rootLogDir, namespace)
	jobs := []params.MachineJob{params.JobManageEnviron}
	values := map[string]string{
		// Delete the obsolete values.
		"_DELETE_": "SHARED_STORAGE_ADDR,SHARED_STORAGE_DIR",
		// Add new values from the environment.
		agent.Namespace: namespace,
		// ContainerType is empty on the bootstrap node.
		agent.ContainerType:    "",
		agent.AgentServiceName: "juju-agent-" + namespace,
		agent.MongoServiceName: "juju-db-" + namespace,
	}

	// We need to create the dirs if they don't exist.
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("cannot create dataDir %q: %v", dataDir, err)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("cannot create logDir %q: %v", logDir, err)
	}

	migrateParams := agent.AgentConfigParams{
		DataDir: dataDir,
		LogDir:  logDir,
		Jobs:    jobs,
		Values:  values,
	}
	return agent.MigrateConfig(context.AgentConfig(), migrateParams)
}
