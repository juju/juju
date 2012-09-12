package container

import (
	"fmt"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/upstart"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Container contains running juju service units.
type Container interface {
	// Deploy deploys the unit into a new container.
	Deploy(unit *state.Unit) error

	// Destroy destroys the unit's container.
	Destroy(unit *state.Unit) error
}

// Simple is a Container that knows how deploy units within	
// the current machine.
type Simple struct {
	DataDir string
	InitDir string
}

// Config holds information about where containers should
// be started.
type Config struct {
	DataDir string
	// InitDir holds the directory where upstart scripts
	// will be deployed. If blank, the system default will
	// be used.
	InitDir string

	// TODO(rog) add LogDir?
}

func deslash(s string) string {
	return strings.Replace(s, "/", "-", -1)
}

func (c *Simple) dirName(unit *state.Unit) string {
	return filepath.Join(c.DataDir, "units", deslash(unit.Name()))
}

func (c *Simple) service(unit *state.Unit) *upstart.Service {
	svc := upstart.NewService("juju-agent-" + deslash(unit.Name()))
	if c.InitDir != "" {
		svc.InitDir = c.InitDir
	}
	return svc
}

func (c *Simple) Deploy(unit *state.Unit) (err error) {
	exe, err := exec.LookPath("jujud")
	if err != nil {
		return fmt.Errorf("cannot find executable: %v", err)
	}
	conf := &upstart.Conf{
		Service: *c.service(unit),
		Desc:    "juju unit agent for " + unit.Name(),
		Cmd:     exe + " unit --unit-name " + unit.Name(),
		// TODO: Out
	}
	dir := c.dirName(unit)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	err = conf.Install()
	if err != nil {
		os.Remove(dir)
		return err
	}
	return nil
}

func (c *Simple) Destroy(unit *state.Unit) error {
	if err := c.service(unit).Remove(); err != nil {
		return err
	}
	return os.RemoveAll(c.dirName(unit))
}
