package ec2

import (
	"launchpad.net/goamz/ec2"
	"launchpad.net/juju/go/environs"
	"net/http"
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

func DeleteStorage(s environs.Storage) error {
	return s.(*storage).deleteAll()
}

func InstanceEC2(inst environs.Instance) *ec2.Instance {
	return inst.(*instance).Instance
}

var origImagesHost = imagesHost

func init() {
	// Make the images data accessible through the "file" protocol.
	http.DefaultTransport.(*http.Transport).RegisterProtocol("file", http.NewFileTransport(http.Dir("images")))
}

func UseTestImageData(local bool) {
	if local {
		imagesHost = "file:"
	} else {
		imagesHost = origImagesHost
	}
}

var originalShortAttempt = shortAttempt
var originalLongAttempt = longAttempt

// ShortTimeouts sets the timeouts to a short period as we
// know that the ec2test server doesn't get better with time,
// and this reduces the test time from 30s to 3s.
func ShortTimeouts(short bool) {
	if short {
		shortAttempt = attemptStrategy{
			total: 0.25e9,
			delay: 0.01e9,
		}
		longAttempt = shortAttempt
	} else {
		shortAttempt = originalShortAttempt
		longAttempt = originalLongAttempt
	}
}

func EC2ErrCode(err error) string {
	return ec2ErrCode(err)
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
