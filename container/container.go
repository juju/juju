package container

import (
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/upstart"
	"os"
	"path/filepath"
	"strings"
)

// Config holds information about where containers should
// be started.
type Config struct {
	VarDir string
	// InitDir holds the directory where upstart scripts
	// will be deployed. If blank, the system default will
	// be used.
	InitDir string

	// TODO(rog) add LogDir?
}

func (c *simple) service(unit *state.Unit) *upstart.Service {
	svc := upstart.NewService("juju-" + unit.AgentName())
	if c.cfg.InitDir != "" {
		svc.InitDir = c.cfg.InitDir
	}
	return svc
}

func (c *simple) dirName(unit *state.Unit) string {
	return filepath.Join(c.cfg.VarDir, "agents", unit.AgentName())
}

// Deploy deploys a unit running the given tools unit into a new container.
// The unit will use the given info to connect to the state.
func Deploy(cfg Config, unit *state.Unit, info *state.Info, tools *state.Tools) (err error) {
	// TODO choose an LXC container when the unit requires isolation.
	cont := &simple{cfg}
	return cont.deploy(unit, info, tools)
}

// Destroy destroys the unit's container.
func Destroy(cfg Config, unit *state.Unit) error {
	cont := &simple{cfg}
	return cont.destroy(unit)
}

// simple knows how deploy units within the current machine.
type simple struct {
	cfg Config
}

func (c *simple) deploy(unit *state.Unit, info *state.Info, tools *state.Tools) (err error) {
	if info.UseSSH {
		return fmt.Errorf("cannot deploy unit agent connecting with ssh")
	}
	toolsDir := environs.AgentToolsDir(c.cfg.VarDir, unit.AgentName())
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
	cmd := fmt.Sprintf(
		"%s unit --zookeeper-servers '%s' --log-file %s --unit-name %s",
		filepath.Join(toolsDir, "jujud"),
		strings.Join(info.Addrs, ","),
		filepath.Join("/var/log/juju", unit.AgentName() + ".log"),
		unit.Name())

	conf := &upstart.Conf{
		Service: *c.service(unit),
		Desc:    "juju unit agent for " + unit.Name(),
		Cmd:     cmd,
	}
	dir := c.dirName(unit)
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
	return conf.Install()
}

func (c *simple) destroy(unit *state.Unit) error {
	if err := c.service(unit).Remove(); err != nil {
		return err
	}
	return os.RemoveAll(c.dirName(unit))
}
