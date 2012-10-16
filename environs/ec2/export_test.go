package ec2

import (
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/s3"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/trivial"
	"net/http"
)

type BootstrapState struct {
	StateInstances []string
}

func LoadState(e environs.Environ) (*BootstrapState, error) {
	s, err := e.(*environ).loadState()
	if err != nil {
		return nil, err
	}
	return &BootstrapState{s.StateInstances}, nil
}

func JujuGroupName(e environs.Environ) string {
	return e.(*environ).jujuGroupName()
}

func MachineGroupName(e environs.Environ, machineId int) string {
	return e.(*environ).machineGroupName(machineId)
}

func EnvironEC2(e environs.Environ) *ec2.EC2 {
	return e.(*environ).ec2()
}

func EnvironS3(e environs.Environ) *s3.S3 {
	return e.(*environ).s3()
}

func DeleteStorageContent(s environs.Storage) error {
	return s.(*storage).deleteAll()
}

func InstanceEC2(inst environs.Instance) *ec2.Instance {
	return inst.(*instance).Instance
}

// BucketStorage returns a storage instance addressing
// an arbitrary s3 bucket.
func BucketStorage(b *s3.Bucket) environs.Storage {
	return &storage{
		bucket: b,
	}
}

var origImagesHost = imagesHost

func init() {
	// Make the images data accessible through the "file" protocol.
	http.DefaultTransport.(*http.Transport).RegisterProtocol("file", http.NewFileTransport(http.Dir("testdata")))
}

func UseTestImageData(local bool) {
	if local {
		imagesHost = "file:"
	} else {
		imagesHost = origImagesHost
	}
}

var origMetadataHost = metadataHost

func UseTestMetadata(local bool) {
	if local {
		metadataHost = "file:"
	} else {
		metadataHost = origMetadataHost
	}
}

var originalShortAttempt = shortAttempt
var originalLongAttempt = longAttempt

// ShortTimeouts sets the timeouts to a short period as we
// know that the ec2test server doesn't get better with time,
// and this reduces the test time from 30s to 3s.
func ShortTimeouts(short bool) {
	if short {
		shortAttempt = trivial.AttemptStrategy{
			Total: 0.25e9,
			Delay: 0.01e9,
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

var MgoPortSuffix = mgoPortSuffix

// FabricateInstance creates a new fictitious instance
// given an existing instance and a new id.
func FabricateInstance(inst environs.Instance, newId string) environs.Instance {
	oldi := inst.(*instance)
	newi := &instance{oldi.e, &ec2.Instance{}}
	*newi.Instance = *oldi.Instance
	newi.InstanceId = newId
	return newi
}
