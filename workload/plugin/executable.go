// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugin

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juju/deputy"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/utils"
	"github.com/juju/juju/workload"
)

const executablePrefix = "juju-workload-"

var executableLogger = loggo.GetLogger("juju.workload.plugin.executable")

// ExecutablePlugin represents a provider for launching, destroying, and
// introspecting workloads via a specific technology such as
// Docker or systemd.
//
// Plugins of this type are expected to handle three commands: launch,
// status, and stop. See the functions of the same name for more
// information about each command.
//
// If the plugin command completes successfully, the plugin should exit
// with a 0 exit code. If there is a problem completing the command, the
// plugin should print the error details to stdout and return a non-zero
// exit code.
//
// Any information written to stderr will be piped to the unit log.
type ExecutablePlugin struct {
	// Name is the name of the plugin.
	Name string
	// Executable is the filename disk where the plugin executable resides.
	Executable string

	// RunCmd runs the provided command.
	RunCmd func(name string, cmd *exec.Cmd) ([]byte, error)
}

func newExecutablePlugin(name, executable string) *ExecutablePlugin {
	p := &ExecutablePlugin{
		Name:       name,
		Executable: executable,
	}

	logger := loggo.GetLogger("juju.workload.plugin." + name)
	p.RunCmd = func(name string, cmd *exec.Cmd) ([]byte, error) {
		return runCmd(name, cmd, func(b []byte) { logger.Infof(string(b)) })
	}

	return p
}

// FindExecutablePlugin returns the plugin for the given name.
func FindExecutablePlugin(name, dataDir string) (*ExecutablePlugin, error) {
	paths := NewPaths(dataDir, name)
	return findExecutablePlugin(name, paths, lookPath)
}

type pluginPaths interface {
	Executable() (string, error)
	Init(string) error
}

func findExecutablePlugin(name string, paths pluginPaths, lookPath func(string) (string, error)) (*ExecutablePlugin, error) {
	executableLogger.Debugf("looking for plugin data for %q in %q", name, paths)
	absPath, err := paths.Executable()
	if errors.IsNotFound(err) {
		executableName := executablePrefix + name
		executableLogger.Debugf("looking on $PATH for plugin executable %q", executableName)
		absPath, err = lookPath(executableName)
		if utils.IsNotFound(err) {
			executableLogger.Debugf("plugin executable %q not found on $PATH (%s)", executableName, os.Getenv("PATH"))
			return nil, errors.NotFoundf("plugin %q", name)
		}
		if err != nil {
			return nil, errors.Trace(err)
		}

		if err := paths.Init(absPath); err != nil {
			executableLogger.Debugf("failed to initialize plugin data in %q to %q", paths, absPath)
			return nil, errors.Trace(err)
		}
		executableLogger.Debugf("initialized plugin data in %q to %q", paths, absPath)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	return newExecutablePlugin(name, absPath), nil
}

// Launch runs the given plugin, passing it the "launch" command, with the
// workload definition passed as json to launch as an argument:
//
//		<plugin> launch <workload-definition>
//
// Workload definition is the serialization of the Workload struct from
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
// The plugin is expected to start the image as a new workload, and write json
// output to stdout.  The form of the output is expected to conform to the
// workload.Details struct.
//
//		{
//			"id" : "some-id", # unique id of the workload
//			"status" : "details" # plugin-specific metadata about the started workload
//		}
//
// The id should be a unique identifier of the workload that the plugin can use
// later to introspect the workload and/or stop it. The contents of status can
// be whatever information the plugin thinks might be relevant to see in the
// service's status output.
func (p ExecutablePlugin) Launch(proc charm.Workload) (workload.Details, error) {
	var details workload.Details
	b, err := json.Marshal(proc)
	if err != nil {
		return details, errors.Annotate(err, "can't convert charm.Workload to json")
	}
	out, err := p.run("launch", string(b))
	if err != nil {
		return details, errors.Trace(err)
	}
	return workload.UnmarshalDetails(out)
}

// Destroy runs the given plugin, passing it the "destroy" command, with the id of the
// workload to destroy as an argument.
//
//		<plugin> destroy <id>
func (p ExecutablePlugin) Destroy(id string) error {
	_, err := p.run("destroy", id)
	return errors.Trace(err)
}

// Status runs the given plugin, passing it the "status" command, with the id of
// the workload to get status about.
//
//		<plugin> status <id>
//
// The plugin is expected to write raw-string status output to stdout if
// successful.
func (p ExecutablePlugin) Status(id string) (workload.PluginStatus, error) {
	out, err := p.run("status", id)
	var status workload.PluginStatus
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
func (p ExecutablePlugin) run(subcommand string, args ...string) ([]byte, error) {
	executableLogger.Debugf("running %s %s %s", p.Executable, subcommand, args)
	cmd := exec.Command(p.Executable, append([]string{subcommand}, args...)...)
	return p.RunCmd(p.Name, cmd)
}

func lookPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return path, err
	}
	return filepath.Abs(path)
}

// runCmd runs the executable at path with the subcommand as the first argument
// and any args in args as further arguments.  It logs to loggo using the name
// as a namespace.
func runCmd(name string, cmd *exec.Cmd, logStderr func(b []byte)) ([]byte, error) {
	stdout := &bytes.Buffer{}
	cmd.Stdout = stdout
	err := deputy.Deputy{
		Errors:    deputy.FromStdout,
		StderrLog: logStderr,
	}.Run(cmd)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return stdout.Bytes(), nil
}
