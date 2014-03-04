// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"text/template"
	"time"

	"launchpad.net/juju-core/utils"
)

var startedRE = regexp.MustCompile(`^.* start/running, process (\d+)\n$`)

// InitDir holds the default init directory name.
var InitDir = "/etc/init"

var InstallStartRetryAttempts = utils.AttemptStrategy{
	Total: 1 * time.Second,
	Delay: 250 * time.Millisecond,
}

// Service provides visibility into and control over an upstart service.
type Service struct {
	Name    string
	InitDir string // defaults to "/etc/init"
}

func NewService(name string) *Service {
	return &Service{Name: name, InitDir: InitDir}
}

// confPath returns the path to the service's configuration file.
func (s *Service) confPath() string {
	return path.Join(s.InitDir, s.Name+".conf")
}

// Installed returns whether the service configuration exists in the
// init directory.
func (s *Service) Installed() bool {
	_, err := os.Stat(s.confPath())
	return err == nil
}

// Running returns true if the Service appears to be running.
func (s *Service) Running() bool {
	cmd := exec.Command("status", "--system", s.Name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return startedRE.Match(out)
}

// Start starts the service.
func (s *Service) Start() error {
	if s.Running() {
		return nil
	}
	err := runCommand("start", "--system", s.Name)
	if err != nil {
		// Double check to see if we were started before our command ran.
		if s.Running() {
			return nil
		}
	}
	return err
}

func runCommand(args ...string) error {
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err == nil {
		return nil
	}
	out = bytes.TrimSpace(out)
	if len(out) > 0 {
		return fmt.Errorf("exec %q: %v (%s)", args, err, out)
	}
	return fmt.Errorf("exec %q: %v", args, err)
}

// Stop stops the service.
func (s *Service) Stop() error {
	if !s.Running() {
		return nil
	}
	return runCommand("stop", "--system", s.Name)
}

// StopAndRemove stops the service and then deletes the service
// configuration from the init directory.
func (s *Service) StopAndRemove() error {
	if !s.Installed() {
		return nil
	}
	if err := s.Stop(); err != nil {
		return err
	}
	return os.Remove(s.confPath())
}

// Remove deletes the service configuration from the init directory.
func (s *Service) Remove() error {
	if !s.Installed() {
		return nil
	}
	return os.Remove(s.confPath())
}

// BUG: %q quoting does not necessarily match libnih quoting rules
// (as used by upstart); this may become an issue in the future.
var confT = template.Must(template.New("").Parse(`
description "{{.Desc}}"
author "Juju Team <juju@lists.ubuntu.com>"
start on runlevel [2345]
stop on runlevel [!2345]
respawn
normal exit 0
{{range $k, $v := .Env}}env {{$k}}={{$v|printf "%q"}}
{{end}}
{{range $k, $v := .Limit}}limit {{$k}} {{$v}}
{{end}}
exec {{.Cmd}}{{if .Out}} >> {{.Out}} 2>&1{{end}}
`[1:]))

// Conf is responsible for defining and installing upstart services. Its fields
// represent elements of an upstart service configuration file.
type Conf struct {
	Service
	// Desc is the upstart service's description.
	Desc string
	// Env holds the environment variables that will be set when the command runs.
	Env map[string]string
	// Limit holds the ulimit values that will be set when the command runs.
	Limit map[string]string
	// Cmd is the command (with arguments) that will be run.
	// The command will be restarted if it exits with a non-zero exit code.
	Cmd string
	// Out, if set, will redirect output to that path.
	Out string
}

// validate returns an error if the service is not adequately defined.
func (c *Conf) validate() error {
	if c.Name == "" {
		return errors.New("missing Name")
	}
	if c.InitDir == "" {
		return errors.New("missing InitDir")
	}
	if c.Desc == "" {
		return errors.New("missing Desc")
	}
	if c.Cmd == "" {
		return errors.New("missing Cmd")
	}
	return nil
}

// render returns the upstart configuration for the service as a string.
func (c *Conf) render() ([]byte, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := confT.Execute(&buf, c); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Install installs and starts the service.
func (c *Conf) Install() error {
	conf, err := c.render()
	if err != nil {
		return err
	}
	if c.Installed() {
		if err := c.StopAndRemove(); err != nil {
			return fmt.Errorf("upstart: could not remove installed service: %s", err)
		}
	}

	if err := ioutil.WriteFile(c.confPath(), conf, 0644); err != nil {
		return err
	}
	// On slower disks, upstart may take a short time to realise
	// that there is a service there.
	for attempt := InstallStartRetryAttempts.Start(); attempt.Next(); {
		if err = c.Start(); err == nil {
			break
		}
	}
	return err
}

// InstallCommands returns shell commands to install and start the service.
func (c *Conf) InstallCommands() ([]string, error) {
	conf, err := c.render()
	if err != nil {
		return nil, err
	}
	return []string{
		fmt.Sprintf("cat >> %s << 'EOF'\n%sEOF\n", c.confPath(), conf),
		"start " + c.Name,
	}, nil
}
