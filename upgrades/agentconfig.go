// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state/api/params"
)

var (
	rootLogDir   = "/var/log"
	rootSpoolDir = "/var/spool/rsyslog"
)

var chownPath = func(path, username string) error {
	user, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("cannot lookup %q user id: %v", username, err)
	}
	uid, err := strconv.Atoi(user.Uid)
	if err != nil {
		return fmt.Errorf("invalid user id %q: %v", user.Uid, err)
	}
	gid, err := strconv.Atoi(user.Gid)
	if err != nil {
		return fmt.Errorf("invalid group id %q: %v", user.Gid, err)
	}
	return os.Chown(path, uid, gid)
}

var isLocalEnviron = func(envConfig *config.Config) bool {
	return envConfig.Type() == "local"
}

func migrateLocalProviderAgentConfig(context Context) error {
	st := context.State()
	if st == nil {
		logger.Debugf("no state connection, no migration required")
		// We're running on a different node than the state server.
		return nil
	}
	envConfig, err := st.EnvironConfig()
	if err != nil {
		return fmt.Errorf("failed to read current config: %v", err)
	}
	if !isLocalEnviron(envConfig) {
		logger.Debugf("not a local environment, no migration required")
		return nil
	}
	attrs := envConfig.AllAttrs()
	rootDir, _ := attrs["root-dir"].(string)
	sharedStorageDir := filepath.Join(rootDir, "shared-storage")
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

	dataDir := rootDir
	localLogDir := filepath.Join(rootDir, "log")
	// rsyslogd is restricted to write to /var/log
	logDir := fmt.Sprintf("%s/juju-%s", rootLogDir, namespace)
	jobs := []params.MachineJob{params.JobManageEnviron}
	values := map[string]string{
		agent.Namespace: namespace,
		// ContainerType is empty on the bootstrap node.
		agent.ContainerType:    "",
		agent.AgentServiceName: "juju-agent-" + namespace,
	}
	deprecatedValues := []string{
		"SHARED_STORAGE_ADDR",
		"SHARED_STORAGE_DIR",
	}

	// Remove shared-storage dir if there.
	if err := os.RemoveAll(sharedStorageDir); err != nil {
		return fmt.Errorf("cannot remove deprecated %q: %v", sharedStorageDir, err)
	}

	// We need to create the dirs if they don't exist.
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("cannot create dataDir %q: %v", dataDir, err)
	}
	// We always recreate the logDir to make sure it's empty.
	if err := os.RemoveAll(logDir); err != nil {
		return fmt.Errorf("cannot remove logDir %q: %v", logDir, err)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("cannot create logDir %q: %v", logDir, err)
	}
	// Reconfigure rsyslog as needed:
	// 1. logDir must be owned by syslog:adm
	// 2. Remove old rsyslog spool config
	// 3. Relink logs to the new logDir
	if err := chownPath(logDir, "syslog"); err != nil {
		return err
	}
	spoolConfig := fmt.Sprintf("%s/machine-0-%s", rootSpoolDir, namespace)
	if err := os.RemoveAll(spoolConfig); err != nil {
		return fmt.Errorf("cannot remove %q: %v", spoolConfig, err)
	}
	allMachinesLog := filepath.Join(logDir, "all-machines.log")
	if err := os.Symlink(allMachinesLog, localLogDir+"/"); err != nil && !os.IsExist(err) {
		return fmt.Errorf("cannot symlink %q to %q: %v", allMachinesLog, localLogDir, err)
	}
	machine0Log := filepath.Join(localLogDir, "machine-0.log")
	if err := os.Symlink(machine0Log, logDir+"/"); err != nil && !os.IsExist(err) {
		return fmt.Errorf("cannot symlink %q to %q: %v", machine0Log, logDir, err)
	}

	newCfg := map[string]interface{}{
		"namespace": namespace,
		"container": container,
	}
	if err := st.UpdateEnvironConfig(newCfg, nil, nil); err != nil {
		return fmt.Errorf("cannot update environment config: %v", err)
	}

	return context.AgentConfig().Migrate(agent.MigrateParams{
		DataDir:      dataDir,
		LogDir:       logDir,
		Jobs:         jobs,
		Values:       values,
		DeleteValues: deprecatedValues,
	})
}
