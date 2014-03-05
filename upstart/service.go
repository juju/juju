// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"fmt"
	"os"
	"os/exec"
	"path"

	"launchpad.net/juju-core/utils"
)

const (
	maxMongoFiles = 65000
	maxAgentFiles = 20000

	// MongoScriptVersion keeps track of changes to the mongo upstart script.
	// Update this version when you update the script that gets installed from
	// MongoUpstartService.
	MongoScriptVersion = 2
)

// JujuMongodPath is the path of the mongod that is bundled specifically for juju.
// This value is public only for testing purposes, please do not change.
var JujuMongodPath = "/usr/lib/juju/bin/mongod"

// MongoPath returns the executable path to be used to run mongod on this machine.
// If the juju-bundled version of mongo exists, it will return that path, otherwise
// it will return the command to run mongod from the path.
func MongodPath() string {
	if _, err := os.Stat(JujuMongodPath); err == nil {
		return JujuMongodPath
	}

	s, _ := exec.LookPath("mongod")

	// just use whatever is in the path
	return s
}

// MongoUpstartService returns the upstart config for the mongo state service.
//
// This method assumes there is a server.pem keyfile in dataDir.
func MongoUpstartService(name, dataDir string, port int) *Conf {
	keyFile := path.Join(dataDir, "server.pem")
	svc := NewService(name)

	dbDir := path.Join(dataDir, "db")

	return &Conf{
		Service: *svc,
		Desc:    "juju state database",
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxMongoFiles, maxMongoFiles),
			"nproc":  fmt.Sprintf("%d %d", maxAgentFiles, maxAgentFiles),
		},
		Cmd: MongodPath() +
			" --auth" +
			" --dbpath=" + dbDir +
			" --sslOnNormalPorts" +
			" --sslPEMKeyFile " + utils.ShQuote(keyFile) +
			" --sslPEMKeyPassword ignored" +
			" --bind_ip 0.0.0.0" +
			" --port " + fmt.Sprint(port) +
			" --noprealloc" +
			" --syslog" +
			" --smallfiles" +
			" --replSet juju",
	}
}

// MachineAgentUpstartService returns the upstart config for a machine agent
// based on the tag and machineId passed in.
func MachineAgentUpstartService(name, toolsDir, dataDir, logDir, tag, machineId string, env map[string]string) *Conf {
	svc := NewService(name)
	logFile := path.Join(logDir, tag+".log")
	// The machine agent always starts with debug turned on.  The logger worker
	// will update this to the system logging environment as soon as it starts.
	return &Conf{
		Service: *svc,
		Desc:    fmt.Sprintf("juju %s agent", tag),
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxAgentFiles, maxAgentFiles),
		},
		Cmd: path.Join(toolsDir, "jujud") +
			" machine" +
			" --data-dir " + utils.ShQuote(dataDir) +
			" --machine-id " + machineId +
			" --debug",
		Out: logFile,
		Env: env,
	}
}
