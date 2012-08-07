package container

import (
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/upstart"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// Container contains running juju service units.
type Container interface {
	// Deploy deploys the unit into a new container.
	Deploy(unit *state.Unit) error

	// Destroy destroys the unit's container.
	Destroy(unit *state.Unit) error

	// ToolsDir returns the slash-separated directory that the tools binaries
	// are stored in for the given unit
	ToolsDir(*state.Unit) string
}

// Simple is an instance of Container that knows how deploy units within
// the current machine.
var Simple Container = simpleContainer{}

// TODO:
//type lxc struct {
//	name string
//}
//
//func LXC(args...) Container {
//}

// upstart uses the system init directory by default. Allow the tests
// to choose a different directory.
var initDir = ""

type simpleContainer struct{}

func deslash(s string) string {
	return strings.Replace(s, "/", "-", -1)
}

func service(unit *state.Unit) *upstart.Service {
	svc := upstart.NewService("juju-agent-" + deslash(unit.Name()))
	svc.InitDir = initDir
	return svc
}

func dirName(unit *state.Unit) string {
	return filepath.FromSlash(path.Join(environs.VarDir, "units", deslash(unit.Name())))
}

func (simpleContainer) Deploy(unit *state.Unit) error {
	exe, err := exec.LookPath("jujud")
	if err != nil {
		return fmt.Errorf("cannot find executable: %v", err)
	}
	conf := &upstart.Conf{
		Service: *service(unit),
		Desc:    "juju unit agent for " + unit.Name(),
		Cmd:     exe + " unit --unit-name " + unit.Name(),
		// TODO: Out
	}
	dir := dirName(unit)
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

func (simpleContainer) Destroy(unit *state.Unit) error {
	svc := service(unit)
	if err := svc.Remove(); err != nil {
		return err
	}
	return os.RemoveAll(dirName(unit))
}

func (simpleContainer) ToolsDir(*state.Unit) string {
	return path.Join(environs.VarDir, "tools")
}
