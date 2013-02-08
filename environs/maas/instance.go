package maas

import (
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
)

type maasInstance struct {
	maasObject *gomaasapi.MAASObject
	environ    *maasEnviron
}

var _ environs.Instance = (*maasInstance)(nil)

func (instance *maasInstance) Id() state.InstanceId {
	return state.InstanceId((*instance.maasObject).URI().String())
}

// refreshInstance refreshes the instance with the most up-to-date information
// from the MAAS server.
func (instance *maasInstance) refreshInstance() error {
	insts, err := instance.environ.Instances([]state.InstanceId{instance.Id()})
	if err != nil {
		return err
	}
	newMaasObject := insts[0].(*maasInstance).maasObject
	instance.maasObject = newMaasObject
	return nil
}

func (instance *maasInstance) DNSName() (string, error) {
	hostname, err := (*instance.maasObject).GetField("hostname")
	if err != nil {
		return "", err
	}
	return hostname, nil
}

func (instance *maasInstance) WaitDNSName() (string, error) {
	return instance.DNSName()
}

func (instance *maasInstance) OpenPorts(machineId string, ports []state.Port) error {
	panic("Not implemented.")
}

func (instance *maasInstance) ClosePorts(machineId string, ports []state.Port) error {
	panic("Not implemented.")
}

func (instance *maasInstance) Ports(machineId string) ([]state.Port, error) {
	panic("Not implemented.")
}
