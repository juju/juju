package container

// Container contains running juju service units.
type Container interface {
	Deploy(unit *state.Unit) error
	Destroy() error
}

// TODO:
//type lxc struct {
//	name string
//}
//
//func LXC(name string) Container {
//}

type simple struct {
	name string
}

func Simple(name string) Container {
	return &simple{name}
}

func (s *simple) Deploy(u *unit.Unit) error {
	up := &upstart.Config{
		Service: upstart.Service {
			Name:
		},
		Desc: "juju unit agent for " + u.Name(),
		
	}
}