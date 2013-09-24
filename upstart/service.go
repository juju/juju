// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"fmt"
	"path"

	"launchpad.net/juju-core/utils"
)

const (
	maxMongoFiles = 65000
	maxAgentFiles = 20000
)

// MongoUpstartService returns the upstart config for the mongo state service.
func MongoUpstartService(name, dataDir, dbDir string, port int) *Conf {
	keyFile := path.Join(dataDir, "server.pem")
	svc := NewService(name)
	return &Conf{
		Service: *svc,
		Desc:    "juju state database",
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxMongoFiles, maxMongoFiles),
			"nproc":  fmt.Sprintf("%d %d", maxAgentFiles, maxAgentFiles),
		},
		Cmd: "/usr/bin/mongod" +
			" --auth" +
			" --dbpath=" + dbDir +
			" --sslOnNormalPorts" +
			" --sslPEMKeyFile " + utils.ShQuote(keyFile) +
			" --sslPEMKeyPassword ignored" +
			" --bind_ip 0.0.0.0" +
			" --port " + fmt.Sprint(port) +
			" --noprealloc" +
			" --syslog" +
			" --smallfiles",
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
