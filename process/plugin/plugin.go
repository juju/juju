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
)

var logger = loggo.GetLogger("juju.process.plugin")

// validationErr represents an error signifying an object with an invalid value.
type validationErr struct {
	*errors.Err
}

// IsInvalid returns whether the given error indicates an invalid value.
func IsInvalid(e error) bool {
	_, ok := e.(validationErr)
	return ok
}

// Find returns the plugin for the given name.
func Find(name string) (*Plugin, error) {
	path, err := exec.LookPath("juju-process-" + name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Plugin{Name: name, Path: path}, nil
}

// ProcDetails represents information about a process launched by a plugin.
type ProcDetails struct {
	// ID is a unique string identifying the process to the plugin.
	ID string `json:"id"`
	// ProcStatus is the status of the process after launch.
	ProcStatus
}

// validate returns nil if this value is valid, and an error that satisfies
// IsValid if it is not.
func (p ProcDetails) validate() error {
	if p.ID == "" {
		e := errors.NewErr("ID cannot be empty")
		return validationErr{&e}
	}
	return p.ProcStatus.validate()
}

// ProcStatus represents the data returned from the Status call.
type ProcStatus struct {
	// Status represents the human-readable string returned by the plugin for
	// the process.
	Status string `json:"status"`
}

// validate returns nil if this value is valid, and an error that satisfies
// IsValid if it is not.
func (p ProcStatus) validate() error {
	if p.Status == "" {
		e := errors.NewErr("Status cannot be empty")
		return validationErr{&e}
	}
	return nil
}

// Plugin represents a provider for launching, destroying, and introspecting
// workload processes via a specific technology such as Docker or systemd.
type Plugin struct {
	// Name is the name of the plugin.
	Name string
	// Path is the filename disk where the plugin executable resides.
	Path string
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
// ProcDetails struct.
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
func (p Plugin) Launch(proc charm.Process) (ProcDetails, error) {
	details := ProcDetails{}
	b, err := json.Marshal(proc)
	if err != nil {
		return details, errors.Annotate(err, "can't convert charm.Process to json")
	}
	out, err := p.run("launch", string(b))
	if err != nil {
		return details, errors.Trace(err)
	}
	return UnmarshalDetails(out)
}

func UnmarshalDetails(b []byte) (ProcDetails, error) {
	details := ProcDetails{}
	if err := json.Unmarshal(b, &details); err != nil {
		return details, errors.Annotate(err, "error parsing data for procdetails")
	}
	if err := details.validate(); err != nil {
		return details, errors.Annotate(err, "invalid procdetails")
	}
	return details, nil

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
func (p Plugin) Status(id string) (ProcStatus, error) {
	out, err := p.run("status", id)
	status := ProcStatus{}
	if err != nil {
		return status, errors.Trace(err)
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return status, errors.Annotatef(err, "error parsing data returned from %q", p.Name)
	}
	if err := status.validate(); err != nil {
		return status, errors.Annotatef(err, "invalid details returned by plugin %q", p.Name)
	}
	return status, nil
}

// run runs the given subcommand of the plugin with the given args.
func (p Plugin) run(subcommand string, args ...string) ([]byte, error) {
	return runCmd(p.Name, p.Path, subcommand, args...)
}

// runCmd runs the executable at path with the subcommand as the first argument
// and any args in args as further arguments.  It logs to loggo using the name
// as a namespace.
var runCmd = func(name, path, subcommand string, args ...string) ([]byte, error) {
	cmd := exec.Command(path, append([]string{subcommand}, args...)...)
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
