package maas

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
)

type maasInstance struct{}

var _ environs.Instance = (*maasInstance)(nil)

func (*maasInstance) Id() state.InstanceId {
	panic("Not implemented.")
}

func (*maasInstance) DNSName() (string, error) {
	panic("Not implemented.")
}

func (*maasInstance) WaitDNSName() (string, error) {
	panic("Not implemented.")
}

func (*maasInstance) OpenPorts(machineId string, ports []state.Port) error {
	panic("Not implemented.")
}

func (*maasInstance) ClosePorts(machineId string, ports []state.Port) error {
	panic("Not implemented.")
}

func (*maasInstance) Ports(machineId string) ([]state.Port, error) {
	panic("Not implemented.")
}
