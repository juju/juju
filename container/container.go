package container

import (
	"fmt"
	"launchpad.net/juju-core/juju/state"
	"launchpad.net/juju-core/juju/upstart"
	"os/exec"
	"strings"
)

// Container contains running juju service units.
type Container interface {
	Deploy() error
	Destroy() error
}

// TODO:
//type lxc struct {
//	name string
//}
//
//func LXC(args...) Container {
//}

// upstart uses /etc/init by default. Allow the tests
// to choose a different directory.
var initDir = ""

type simple struct {
	unit *state.Unit
}

// Installer is used to install simple containers.
// The upstart installer is used by default.
var SimpleInstaller = (*upstart.Conf).Install

func Simple(unit *state.Unit) Container {
	return &simple{unit}
}

func deslash(s string) string {
	return strings.Replace(s, "/", "-", -1)
}

func (s *simple) service() *upstart.Service {
	svc := upstart.NewService("juju-agent-" + deslash(s.unit.Name()))
	svc.InitDir = initDir
	return svc
}

func (s *simple) Deploy() error {
	exe, err := exec.LookPath("jujud")
	if err != nil {
		return fmt.Errorf("cannot find executable: %v", err)
	}
	conf := &upstart.Conf{
		Service: *s.service(),
		Desc:    "juju unit agent for " + s.unit.Name(),
		Cmd:     exe + " unit --unit-name " + s.unit.Name(),
		// TODO: Out
	}
	return SimpleInstaller(conf)
}

func (s *simple) Destroy() error {
	// TODO what, if any, directory do we need to delete?
	return s.service().Remove()
}

