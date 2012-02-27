package ec2

import (
	"launchpad.net/goamz/ec2"
	"launchpad.net/juju/go/environs"
)

type BootstrapState struct {
	ZookeeperInstances []string
}

func LoadState(e environs.Environ) (*BootstrapState, error) {
	s, err := e.(*environ).loadState()
	if err != nil {
		return nil, err
	}
	return &BootstrapState{s.ZookeeperInstances}, nil
}

func GroupName(e environs.Environ) string {
	return e.(*environ).groupName()
}

func MachineGroupName(e environs.Environ, machineId int) string {
	return e.(*environ).machineGroupName(machineId)
}

func AuthorizedKeys(path string) (string, error) {
	return authorizedKeys(path)
}

func EnvironEC2(e environs.Environ) *ec2.EC2 {
	return e.(*environ).ec2
}

// FabricateInstance creates a new fictitious instance
// given an existing instance and a new id.
func FabricateInstance(inst environs.Instance, newId string) environs.Instance {
	oldi := inst.(*instance)
	newi := &instance{&ec2.Instance{}}
	*newi.Instance = *oldi.Instance
	newi.InstanceId = newId
	return newi
}
