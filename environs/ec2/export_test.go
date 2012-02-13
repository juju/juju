package ec2

import (
	"launchpad.net/juju/go/environs"
	"launchpad.net/goamz/ec2"
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

func AuthorizedKeys(keys, path string) (string, error) {
	return authorizedKeys(keys, path)
}


func EnvironEC2(e environs.Environ) *ec2.EC2 {
	return e.(*environ).ec2
}

var ZkPortSuffix = zkPortSuffix
