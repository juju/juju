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

func InstanceEC2(inst environs.Instance) *ec2.Instance {
	return inst.(*instance).Instance
}

var originalShortAttempt = shortAttempt
var originalLongAttempt = longAttempt

// ShortTimeouts sets the timeouts to a short period as we
// know that the ec2test server doesn't get better with time,
// and this reduces the test time from 30s to 3s.
func ShortTimeouts(short bool) {
	if short {
		shortAttempt = attempt{
			burstTotal: 0.25e9,
			burstDelay: 0.01e9,
		}
		longAttempt = shortAttempt
	} else {
		shortAttempt = originalShortAttempt
		longAttempt = originalLongAttempt
	}
}

func LongDo(t func(error) bool, f func() error) error {
	return longAttempt.do(t, f)
}

func ShortDo(t func(error) bool, f func() error) error {
	return shortAttempt.do(t, f)
}

func EC2ErrCode(err error) string {
	return ec2ErrCode(err)
}

func HasCode(code string) func(error) bool {
	return hasCode(code)
}

var ZkPortSuffix = zkPortSuffix

// FabricateInstance creates a new fictitious instance
// given an existing instance and a new id.
func FabricateInstance(inst environs.Instance, newId string) environs.Instance {
	oldi := inst.(*instance)
	newi := &instance{oldi.e, &ec2.Instance{}}
	*newi.Instance = *oldi.Instance
	newi.InstanceId = newId
	return newi
}
