// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package plugin contains the code that interfaces with plugins for workload
// process technologies such as Docker, Rocket, or systemd.
//
// Plugins of this type are expected to handle three commands: launch, status,
// and stop.  See the functions of the same name for more information about each
// command.
//
// If the plugin command completes successfully, the plugin should exit with a
// 0 exit code. If there is a problem completing the command, the plugin should
// print the error details to stdout and return a non-zero exit code.
//
// Any information written to stderr will be piped to the unit log.
package plugin

import (
	"bytes"
	"encoding/json"
	"os/exec"

	"github.com/juju/deputy"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
)

const pluginPrefix = "juju-process-"

var logger = loggo.GetLogger("juju.process.plugin")

// Plugin represents a provider for launching, destroying, and introspecting
// workload processes via a specific technology such as Docker or systemd.
type Plugin struct {
	// Name is the name of the plugin.
	Name string
	// Executable is the filename disk where the plugin executable resides.
	Executable string
}

// Find returns the plugin for the given name.
func Find(name string) (*Plugin, error) {
	path, err := exec.LookPath(pluginPrefix + name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Plugin{Name: name, Executable: path}, nil
}

// Launch runs the given plugin, passing it the "launch" command, with the
// process definition passed as json to launch as an argument:
//
//		<plugin> launch <process-definition>
//
// Process definition is the serialization of the Process struct from
// github.com/juju/charm, for example:
//
//		{
//			"name" : "procname",
//			"description" : "desc",
//			"type" : "proctype",
//			"typeOptions" : {
//				"key1" : "val1"
//			},
//			"command" : "cmd",
//			"image" : "img",
//			"ports" : [ { "internal" : 80, "external" : 8080 } ]
//			"volumes" : [{"externalmount" : "mnt", "internalmount" : "extmnt", "mode" : "rw", "name" : "name"}]
//			"envvars" : {
//				"key1" : "val1"
//			}
//		}
//
// The plugin is expected to start the image as a new process, and write json
// output to stdout.  The form of the output is expected to conform to the
// process.Details struct.
//
//		{
//			"id" : "some-id", # unique id of the process
//			"status" : "details" # plugin-specific metadata about the started process
//		}
//
// The id should be a unique identifier of the process that the plugin can use
// later to introspect the process and/or stop it. The contents of status can
// be whatever information the plugin thinks might be relevant to see in the
// service's status output.
func (p Plugin) Launch(proc charm.Process) (process.Details, error) {
	var details process.Details
	b, err := json.Marshal(proc)
	if err != nil {
		return details, errors.Annotate(err, "can't convert charm.Process to json")
	}
	out, err := p.run("launch", string(b))
	if err != nil {
		return details, errors.Trace(err)
	}
	return process.UnmarshalDetails(out)
}

// Destroy runs the given plugin, passing it the "destroy" command, with the id of the
// process to destroy as an argument.
//
//		<plugin> destroy <id>
func (p Plugin) Destroy(id string) error {
	_, err := p.run("destroy", id)
	return errors.Trace(err)
}

// Status runs the given plugin, passing it the "status" command, with the id of
// the process to get status about.
//
//		<plugin> status <id>
//
// The plugin is expected to write raw-string status output to stdout if
// successful.
func (p Plugin) Status(id string) (process.PluginStatus, error) {
	out, err := p.run("status", id)
	var status process.PluginStatus
	if err != nil {
		return status, errors.Trace(err)
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return status, errors.Annotatef(err, "error parsing data returned from %q", p.Name)
	}
	if err := status.Validate(); err != nil {
		return status, errors.Annotatef(err, "invalid details returned by plugin %q", p.Name)
	}
	return status, nil
}

// run runs the given subcommand of the plugin with the given args.
func (p Plugin) run(subcommand string, args ...string) ([]byte, error) {
	logger.Debugf("running %s %s %s", p.Executable, subcommand, args)
	cmd := exec.Command(p.Executable, append([]string{subcommand}, args...)...)
	return runCmd(p.Name, cmd)
}

// runCmd runs the executable at path with the subcommand as the first argument
// and any args in args as further arguments.  It logs to loggo using the name
// as a namespace.
var runCmd = func(name string, cmd *exec.Cmd) ([]byte, error) {
	log := getLogger("juju.process.plugin." + name)
	stdout := &bytes.Buffer{}
	cmd.Stdout = stdout
	err := deputy.Deputy{
		Errors:    deputy.FromStdout,
		StderrLog: func(b []byte) { log.Infof(string(b)) },
	}.Run(cmd)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return stdout.Bytes(), nil
}

type infoLogger interface {
	Infof(s string, args ...interface{})
}

var getLogger = func(name string) infoLogger {
	return loggo.GetLogger(name)
}
