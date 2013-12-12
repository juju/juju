// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"net/rpc"
	"os"
	"path/filepath"

	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/worker/uniter"
)

var AgentDir = "/var/lib/juju/agents"

const usage = `juju-run <unit-name> <commands>

unit-name can be either the unit tag:
 i.e.  unit-ubuntu-0
or the unit id:
 i.e.  ubuntu/0

The commands are executed with '/bin/bash -s', and the output returned.
`

func printUsage() {
	os.Stdout.Write([]byte(usage))
}

func jujuRun(args []string) (code int, err error) {
	code = 1
	// make sure we aren't in an existing hook context
	if contextId, err := getenv("JUJU_CONTEXT_ID"); err == nil && contextId != "" {
		return code, fmt.Errorf("juju-run cannot be called from within a hook, have context %q", contextId)
	}

	if len(args) < 1 {
		printUsage()
		return code, fmt.Errorf("missing unit-name")
	}
	if len(args) < 2 {
		printUsage()
		return code, fmt.Errorf("missing commands")
	}
	if len(args) > 2 {
		printUsage()
		return code, fmt.Errorf("too many arguments")
	}
	unit := args[0]
	if names.IsUnit(unit) {
		unit = names.UnitTag(unit)
	}
	commands := args[1]

	unitDir := filepath.Join(AgentDir, unit)
	logger.Debugf("looking for unit dir %s", unitDir)
	// make sure the unit exists
	fileInfo, err := os.Stat(unitDir)
	if os.IsNotExist(err) {
		return code, fmt.Errorf("unit %q not found on this machine", unit)
	} else if err != nil {
		return code, err
	}
	if !fileInfo.IsDir() {
		return code, fmt.Errorf("%q is not a directory", unitDir)
	}

	socketPath := filepath.Join(unitDir, uniter.RunListenerFile)
	// make sure the socket exists
	client, err := rpc.Dial("unix", socketPath)
	if err != nil {
		return
	}
	defer client.Close()

	var result uniter.RunResults
	err = client.Call("Runner.RunCommands", commands, &result)
	if err != nil {
		return
	}

	os.Stdout.Write([]byte(result.StdOut))
	os.Stderr.Write([]byte(result.StdErr))
	return result.ReturnCode, nil
}
