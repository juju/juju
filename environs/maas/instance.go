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

func (inst *maasInstance) Id() state.InstanceId {
	return state.InstanceId((*inst.maasObject).URI().String())
}

// refreshInstance refreshes the instance with the most up-to-date information
// from the MAAS server.
func (inst *maasInstance) refreshInstance() error {
	insts, err := inst.environ.Instances([]state.InstanceId{inst.Id()})
	if err != nil {
		return err
	}
	newMaasObject := insts[0].(*maasInstance).maasObject
	inst.maasObject = newMaasObject
	return nil
}

func (inst *maasInstance) DNSName() (string, error) {
	err := inst.refreshInstance()
	if err != nil {
		return "", err
	}
	hostname, err := (*inst.maasObject).GetField("hostname")
	if err != nil {
		return "", err
	}
	return hostname, nil
}

func (inst *maasInstance) WaitDNSName() (string, error) {
	return inst.DNSName()
}

func (inst *maasInstance) OpenPorts(machineId string, ports []state.Port) error {
	panic("Not implemented.")
}

func (inst *maasInstance) ClosePorts(machineId string, ports []state.Port) error {
	panic("Not implemented.")
}

func (inst *maasInstance) Ports(machineId string) ([]state.Port, error) {
	panic("Not implemented.")
}
