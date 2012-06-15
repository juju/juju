package container
import (
	"launchpad.net/juju-core/juju/state"
	"strings"
	"launchpad.net/juju-core/juju/upstart"
	"os/exec"
	"fmt"
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

type simple struct {
	unit *state.Unit
}

func Simple(unit *state.Unit) Container {
	return &simple{unit}
}

func deslash(s string) string {
	return strings.Replace(s, "/", "-", -1)
}

func (s *simple) service() *upstart.Service {
	return upstart.NewService(deslash(s.unit.Name()))
}

func (s *simple) Deploy() error {
	exe, err := exec.LookPath("jujud")
	if err != nil {
		return fmt.Errorf("cannot find executable: %v", err)
	}
	conf := &upstart.Conf {
		Service: *s.service(),
		Desc: "juju unit agent for " + s.unit.Name(),
		Cmd: exe+" unit --unit-name " + s.unit.Name(),
		// TODO: Out
	}
	return conf.Install()
}

func (s *simple) Destroy() error {
	// TODO what, if any, directory do we need to delete?
	return s.service().Remove()
}
