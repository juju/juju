// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// package plugin contains the code that interfaces with plugins for workload
// process technologies such as Docker, Rocket, or systemd.
//
// Plugins of this type are expected to handle three commands: launch, status,
// and stop.  See the functions of the same name for more information about each
// command.
//
// If the plugin command  completes successfully, the plugin should exit with a
// 0 exit code. If there is a problem completing the command, the plugin should
// print the error details to stdout and return a non-zero exit code.
//
// If the plugin takes more than 30 seconds to execute its task, it will be
// killed and an error returned.
//
// Any information written to stderr will be piped to the unit log.
package plugin

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/yaml.v2"
)

const timeout = time.Second * 30

type timeoutErr struct {
	*errors.Err
}

func (t timeoutErr) IsTimeout() bool {
	return true
}

// ProcDetails represents information about a process launched by a plugin.
type ProcDetails struct {
	ID      string                 `yaml:"id"`
	Details map[string]interface{} `yaml:"details"`
}

// Launch runs the given plugin, passing it the "launch" command, with the path
// to the image to launch as an argument.
//
// 		launch <image>
//
// The plugin is expected to start the image as a new process, and write yaml
// output to stdout.  The form of the output is expected to be:
//
// 		id: "some-id" # unique id of the process
//		details:      # plugin-specific metadata about the started process
//			foo: bar
//			baz: 5
//
// The id should be a unique identifier of the process that the plugin can use
// later to introspect the process and/or stop it. The contents of details can
// be whatever information the plugin thinks might be relevant to see in the
// service's status output.
func Launch(plugin, image string) (ProcDetails, error) {
	cmd := exec.Command(plugin, "launch", image)
	cmd.Stderr = logwriter{loggo.GetLogger("juju.process.plugin." + filepath.Base(plugin))}
	stdout, err := outputWithTimeout(cmd, timeout)
	p := ProcDetails{}
	if err != nil {
		return p, err
	}
	if err := yaml.Unmarshal(stdout, &p); err != nil {
		return p, errors.Annotatef(err, "error parsing data returned from %q", plugin)
	}
	if p.ID == "" {
		return p, errors.Errorf("no id set by plugin %q", plugin)
	}
	return p, nil
}

// Stop runs the given plugin, passing it the "stop" command, with the id of the
// process to stop as an argument.
//
// 		stop <id>
func Stop(plugin, id string) error {
	cmd := exec.Command(plugin, "stop", id)
	cmd.Stderr = logwriter{loggo.GetLogger("juju.process.plugin." + filepath.Base(plugin))}
	_, err := outputWithTimeout(cmd, timeout)
	return err
}

// Status runs the given plugin, passing it the "status" command, with the id of
// the process to get status about.
//
// 		status <id>
//
// The plugin is expected to write raw-string status output to stdout if
// successful.
func Status(plugin, id string) (string, error) {
	cmd := exec.Command(plugin, "status", id)
	cmd.Stderr = logwriter{loggo.GetLogger("juju.process.plugin." + filepath.Base(plugin))}
	stdout, err := outputWithTimeout(cmd, timeout)
	if err != nil {
		return "", err
	}
	return string(stdout), nil
}

// outputWithTimeout runs the Cmd and kills if it takes longer than
func outputWithTimeout(cmd *exec.Cmd, timeout time.Duration) ([]byte, error) {
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	done := make(chan error)

	var err error
	go func() {
		err = cmd.Wait()
		close(done)
	}()

	select {
	case <-time.After(timeout):
		cmd.Process.Kill()
		err := errors.NewErr("timed out waiting for plugin %q to execute", cmd.Path)
		return nil, timeoutErr{&err}
	case <-done:
	}

	if err != nil {
		if stdout.Len() > 0 {
			return nil, errors.Wrap(err, errors.New(stdout.String()))
		}
		return nil, err
	}

	return stdout.Bytes(), nil
}

// logwriter is a little helper that can be used as an io.Writer and writes to a
// loggo.Logger.
type logwriter struct {
	loggo.Logger
}

// Write implements io.Write.
func (l logwriter) Write(b []byte) (n int, err error) {
	l.Infof(string(b))
	return len(b), nil
}
