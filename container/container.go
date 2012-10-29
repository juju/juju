package container

import (
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/upstart"
	"os"
	"path/filepath"
	"strings"
)

// Container contains running juju service units.
type Container interface {
	// Deploy deploys the unit into a new container.
	Deploy(unit *state.Unit, info *state.Info, tools *state.Tools) error

	// Destroy destroys the unit's container.
	Destroy(unit *state.Unit) error
}

// Simple is a Container that knows how deploy units within
// the current machine.
type Simple struct {
	DataDir string
	// InitDir holds the directory where upstart scripts
	// will be deployed. If blank, the system default will
	// be used.
	InitDir string

	// TODO(rog) add LogDir?
}

func (c *Simple) service(unit *state.Unit) *upstart.Service {
	svc := upstart.NewService("juju-" + unit.EntityName())
	if c.InitDir != "" {
		svc.InitDir = c.InitDir
	}
	return svc
}

// Deploy deploys a unit running the given tools unit into a new container.
// The unit will use the given info to connect to the state.
func (c *Simple) Deploy(unit *state.Unit, info *state.Info, tools *state.Tools) (err error) {
	if info.UseSSH {
		return fmt.Errorf("cannot deploy unit agent connecting with ssh")
	}
	toolsDir := environs.AgentToolsDir(c.DataDir, unit.EntityName())
	err = os.Symlink(tools.Binary.String(), toolsDir)
	if err != nil {
		return fmt.Errorf("cannot make agent tools symlink: %v", err)
	}
	defer func() {
		if err != nil {
			if err := os.Remove(toolsDir); err != nil {
				log.Printf("container: cannot remove tools symlink: %v", err)
			}
		}
	}()
	password, err := trivial.RandomPassword()
	if err != nil {
		return fmt.Errorf("cannot make password for unit: %v", err)
	}
	debugFlag := ""
	// TODO: disable debug mode by default when the system is stable.
	if true || log.Debug {
		debugFlag = " --debug"
	}
	logPath := filepath.Join("/var/log/juju", unit.EntityName()+".log")
	cmd := fmt.Sprintf(
		"%s unit"+
			"%s --state-servers '%s'"+
			" --log-file %s"+
			" --unit-name %s"+
			" --initial-password %s",
		filepath.Join(toolsDir, "jujud"),
		debugFlag,
		strings.Join(info.Addrs, ","),
		logPath,
		unit.Name(),
		password)

	conf := &upstart.Conf{
		Service: *c.service(unit),
		Desc:    "juju unit agent for " + unit.Name(),
		Cmd:     cmd,
		Out:     logPath,
	}
	dir := environs.AgentDir(c.DataDir, unit.EntityName())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if err := os.Remove(dir); err != nil {
				log.Printf("container: cannot remove agent dir: %v", err)
			}
		}
	}()

	if err := unit.SetPassword(password); err != nil {
		return fmt.Errorf("cannot set password for unit: %v", err)
	}
	return conf.Install()
}

// Destroy destroys the unit's container.
func (c *Simple) Destroy(unit *state.Unit) error {
	if err := c.service(unit).Remove(); err != nil {
		return err
	}
	return os.RemoveAll(environs.AgentDir(c.DataDir, unit.EntityName()))
}
