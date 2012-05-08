package upstart

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var startedRE = regexp.MustCompile("^.* start/running, process (\\d+)\n$")

// Service provides visibility into and control over an upstart service.
type Service struct {
	Name    string
	InitDir string // defaults to "/etc/init"
}

func NewService(name string) *Service {
	return &Service{Name: name, InitDir: "/etc/init"}
}

// path returns the path to the service's configuration file.
func (s *Service) path() string {
	return filepath.Join(s.InitDir, s.Name+".conf")
}

// pid returns the Service's current pid, or -1 if it cannot be determined.
func (s *Service) pid() int {
	cmd := exec.Command("status", s.Name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return -1
	}
	match := startedRE.FindStringSubmatch(string(out))
	if match == nil {
		return -1
	}
	pid, err := strconv.Atoi(match[1])
	if err != nil {
		return -1
	}
	return pid
}

// Installed returns true if the Service appears to be installed.
func (s *Service) Installed() bool {
	_, err := os.Stat(s.path())
	return err == nil
}

// Running returns true if the Service appears to be running.
func (s *Service) Running() bool {
	return s.pid() != -1
}

// Stable returns true if the Service appears to be running stably, by
// checking that the reported pid does not change over the course of 5
// checks over 0.4 seconds.
func (s *Service) Stable() bool {
	pid := s.pid()
	if pid == -1 {
		return false
	}
	for i := 0; i < 4; i++ {
		<-time.After(100 * time.Millisecond)
		if s.pid() != pid {
			return false
		}
	}
	return true
}

// Start starts the service.
func (s *Service) Start() error {
	if s.Running() {
		return nil
	}
	return exec.Command("start", s.Name).Run()
}

// Stop stops the service.
func (s *Service) Stop() error {
	if !s.Running() {
		return nil
	}
	return exec.Command("stop", s.Name).Run()
}

// Remove removes the service.
func (s *Service) Remove() error {
	if !s.Installed() {
		return nil
	}
	if err := s.Stop(); err != nil {
		return err
	}
	return os.Remove(s.path())
}

const (
	descT = `description "%s"
author "Juju Team <juju@lists.ubuntu.com>"
`
	start = `start on runlevel [2345]
stop on runlevel [!2345]
respawn
`
	envT  = "env %s=%q\n"
	outT  = "/tmp/%s.output"
	execT = "exec %s >> %s 2>&1\n"
)

// Conf is responsible for defining and installing upstart services.
type Conf struct {
	Service
	Desc string
	Env  map[string]string
	Cmd  string
	Out  string
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
func (c *Conf) render() (string, error) {
	if err := c.validate(); err != nil {
		return "", err
	}
	parts := []string{fmt.Sprintf(descT, c.Desc), start}
	for k, v := range c.Env {
		parts = append(parts, fmt.Sprintf(envT, k, v))
	}
	out := c.Out
	if out == "" {
		out = fmt.Sprintf(outT, c.Name)
	}
	parts = append(parts, fmt.Sprintf(execT, c.Cmd, out))
	return strings.Join(parts, ""), nil
}

// Install installs and starts the service.
func (c *Conf) Install() error {
	conf, err := c.render()
	if err != nil {
		return err
	}
	if c.Installed() {
		if err := c.Stop(); err != nil {
			return err
		}
	}
	if err := ioutil.WriteFile(c.path(), []byte(conf), 0644); err != nil {
		return err
	}
	return c.Start()
}

// InstallCommands returns shell commands to install and start the service.
func (c *Conf) InstallCommands() ([]string, error) {
	conf, err := c.render()
	if err != nil {
		return nil, err
	}
	return []string{
		fmt.Sprintf("cat >> %c << EOF\n%sEOF\n", c.path(), conf),
		"start " + c.Name,
	}, nil
}
