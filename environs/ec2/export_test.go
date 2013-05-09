package ec2

import (
	"io"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/s3"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/utils"
	"net/http"
)

type BootstrapState struct {
	StateInstances []state.InstanceId
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

func MachineGroupName(e environs.Environ, machineId string) string {
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

var testRoundTripper = &jujutest.ProxyRoundTripper{}

func init() {
	// Prepare mock http transport for overriding metadata and images output in tests
	http.DefaultTransport.(*http.Transport).RegisterProtocol("test", testRoundTripper)
}

// TODO: Apart from overriding different hardcoded hosts, these two test helpers are identical. Let's share.

var origImagesHost = imagesHost

// UseTestImageData causes the given content to be served
// when the ec2 client asks for image data.
func UseTestImageData(content []jujutest.FileContent) {
	if content != nil {
		testRoundTripper.Sub = jujutest.NewVirtualRoundTripper(content)
		imagesHost = "test:"
	} else {
		testRoundTripper.Sub = nil
		imagesHost = origImagesHost
	}
}

// UseTestInstanceTypeData causes the given instance type
// cost data to be served for the "test" region.
func UseTestInstanceTypeData(content instanceTypeCost) {
	if content != nil {
		allRegionCosts["test"] = content
	} else {
		delete(allRegionCosts, "test")
	}
}

var origMetadataHost = metadataHost

func UseTestMetadata(content []jujutest.FileContent) {
	if content != nil {
		testRoundTripper.Sub = jujutest.NewVirtualRoundTripper(content)
		metadataHost = "test:"
	} else {
		testRoundTripper.Sub = nil
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
		shortAttempt = utils.AttemptStrategy{
			Total: 0.25e9,
			Delay: 0.01e9,
		}
		longAttempt = shortAttempt
	} else {
		shortAttempt = originalShortAttempt
		longAttempt = originalLongAttempt
	}
}

var ShortAttempt = &shortAttempt

func EC2ErrCode(err error) string {
	return ec2ErrCode(err)
}

// FabricateInstance creates a new fictitious instance
// given an existing instance and a new id.
func FabricateInstance(inst environs.Instance, newId string) environs.Instance {
	oldi := inst.(*instance)
	newi := &instance{oldi.e, &ec2.Instance{}}
	*newi.Instance = *oldi.Instance
	newi.InstanceId = newId
	return newi
}

// Access non exported methods on ec2.storage
type Storage interface {
	Put(file string, r io.Reader, length int64) error
	ResetMadeBucket()
}

func (s *storage) ResetMadeBucket() {
	s.Lock()
	defer s.Unlock()
	s.madeBucket = false
}

// WritablePublicStorage returns a Storage instance which is authorised to write to the PublicStorage bucket.
// It is used by tests which need to upload files.
func WritablePublicStorage(e environs.Environ) environs.Storage {
	// In the case of ec2, access to the public storage instance is created with the user's AWS credentials.
	// So write access is there implicitly, and we just need to cast to a writable storage instance.
	// This contrasts with the openstack case, where the public storage instance truly is read only and we need
	// to create a separate writable instance. If the ec2 case ever changes, the changes are confined to this method.
	return e.PublicStorage().(environs.Storage)
}
