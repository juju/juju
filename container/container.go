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

func deslash(s string) string {
	return strings.Replace(s, "/", "-", -1)
}

func (c *simple) dirName(unit *state.Unit) string {
	return filepath.Join(c.cfg.VarDir, "units", deslash(unit.Name()))
}

func (c *simple) service(unit *state.Unit) *upstart.Service {
	svc := upstart.NewService("juju-agent-" + deslash(unit.Name()))
	if c.cfg.InitDir != "" {
		svc.InitDir = c.cfg.InitDir
	}
	return svc
}

// Deploy deploys the unit into a new container.
func Deploy(cfg Config, unit *state.Unit) (err error) {
	// TODO choose an LXC container when the unit requires isolation.
	cont := &simple{cfg}
	return cont.deploy(unit)
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

func (c *simple) deploy(unit *state.Unit) (err error) {
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

func (c *simple) destroy(unit *state.Unit) error {
	if err := c.service(unit).Remove(); err != nil {
		return err
	}
	return os.RemoveAll(c.dirName(unit))
}
