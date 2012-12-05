package api
import (
	"fmt"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/environs/config"
	"net"
)

type State struct {
	c *rpc.Client
}

type Info struct {
	Addr string
	EntityName string
	Password string
}

func Open(info *Info) (*State, error) {
	c, err := net.Dial("tcp", info.Addr)
	if err != nil {
		return nil, err
	}
	client := rpc.NewClientWithCodec(rpc.NewJSONClientCodec(c))
	// TODO authenticate with entity name and password
	return &State{client}, nil
}

type Machine struct {
	state *State
	Id string
}

func (s *State) Machine(id string) (*Machine, error) {
	var m Machine
	if err := s.c.Call("/Machine", id, &m); err != nil {
		return nil, err
	}
	m.state = s
	return &m, nil
}

func (s *State) AllMachines() ([]*Machine, error) {
	var ms []*Machine
	
	if err := s.c.Call("/AllMachines", nil, &ms); err != nil {
		return nil, err
	}
	for i := range ms {
		ms[i].state = s
	}
	return ms, nil
}

func (s *State) AddMachine() (*Machine, error) {
	var m Machine
	if err := s.c.Call("/AddMachine", nil, &m); err != nil {
		return nil, err
	}
	m.state = s
	return &m, nil
}

func (s *State) EnvironConfig() (*config.Config, error) {
	var attrs map[string]interface{}
	if err := s.c.Call("/EnvironConfig", nil, &attrs); err != nil {
		return nil, err
	}
	return config.New(attrs)
}

type InstanceId string

func (m *Machine) InstanceId() (InstanceId, error) {
	path := fmt.Sprintf("/Machine-%s/InstanceId", m.Id)
	var id InstanceId
	if err := m.state.c.Call(path, nil, &id); err != nil {
		return "", err
	}
	return id, nil
}

func (m *Machine) SetInstanceId(instId InstanceId) error {
	path := fmt.Sprintf("/Machine-%s/SetInstanceId", m.Id)
	return m.state.c.Call(path, instId, nil)
}
