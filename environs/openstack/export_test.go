package openstack

import (
	"fmt"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/swift"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
	"net/http"
	"time"
)

// This provides the content for code accessing test:///... URLs. This allows
// us to set the responses for things like the Metadata server, by pointing
// metadata requests at test:///... rather than http://169.254.169.254
var testRoundTripper = &jujutest.ProxyRoundTripper{}

func init() {
	http.DefaultTransport.(*http.Transport).RegisterProtocol("test", testRoundTripper)
}

var origMetadataHost = metadataHost

var metadataContent = `{"uuid": "d8e02d56-2648-49a3-bf97-6be8f1204f38",` +
	`"availability_zone": "nova", "hostname": "test.novalocal", ` +
	`"launch_index": 0, "meta": {"priority": "low", "role": "webserver"}, ` +
	`"public_keys": {"mykey": "ssh-rsa fake-key\n"}, "name": "test"}`

// A group of canned responses for the "metadata server". These match
// reasonably well with the results of making those requests on a Folsom+
// Openstack service
var MetadataTestingBase = []jujutest.FileContent{
	{"/latest/meta-data/instance-id", "i-000abc"},
	{"/latest/meta-data/local-ipv4", "10.1.1.2"},
	{"/latest/meta-data/public-ipv4", "203.1.1.2"},
	{"/openstack/2012-08-10/meta_data.json", metadataContent},
}

// This is the same as MetadataTestingBase, but it doesn't have the openstack
// 2012-08-08 API. This matches what is available in HP Cloud.
var MetadataHP = MetadataTestingBase[:len(MetadataTestingBase)-1]

// Set Metadata requests to be served by the filecontent supplied.
func UseTestMetadata(metadata []jujutest.FileContent) {
	if len(metadata) != 0 {
		testRoundTripper.Sub = jujutest.NewVirtualRoundTripper(metadata)
		metadataHost = "test:"
	} else {
		testRoundTripper.Sub = nil
		metadataHost = origMetadataHost
	}
}

var originalShortAttempt = shortAttempt
var originalLongAttempt = longAttempt

// ShortTimeouts sets the timeouts to a short period as we
// know that the testing server doesn't get better with time,
// and this reduces the test time from 30s to 3s.
func ShortTimeouts(short bool) {
	if short {
		shortAttempt = trivial.AttemptStrategy{
			Total: 100 * time.Millisecond,
			Delay: 10 * time.Millisecond,
		}
		longAttempt = shortAttempt
	} else {
		shortAttempt = originalShortAttempt
		longAttempt = originalLongAttempt
	}
}

var ShortAttempt = &shortAttempt

func DeleteStorageContent(s environs.Storage) error {
	return s.(*storage).deleteAll()
}

// WritablePublicStorage returns a Storage instance which is authorised to write to the PublicStorage bucket.
// It is used by tests which need to upload files.
func WritablePublicStorage(e environs.Environ) environs.Storage {
	ecfg := e.(*environ).ecfg()
	authModeCfg := AuthMode(ecfg.authMode())
	writablePublicStorage := &storage{
		containerName: ecfg.publicBucket(),
		swift:         swift.New(e.(*environ).client(ecfg, authModeCfg)),
	}

	// Ensure the container exists.
	err := writablePublicStorage.makeContainer(ecfg.publicBucket(), swift.PublicRead)
	if err != nil {
		panic(fmt.Errorf("cannot create writable public container: %v", err))
	}
	return writablePublicStorage
}
func InstanceAddress(addresses map[string][]nova.IPAddress) (string, error) {
	return instanceAddress(addresses)
}

func FindInstanceSpec(e environs.Environ, possibleTools tools.List) (imageId, flavorId string, tools *state.Tools, err error) {
	spec, err := findInstanceSpec(e.(*environ), possibleTools)
	if err == nil {
		imageId = spec.imageId
		flavorId = spec.flavorId
		tools = spec.tools
	}
	return
}

func SetUseFloatingIP(e environs.Environ, val bool) {
	env := e.(*environ)
	env.ecfg().attrs["use-floating-ip"] = val
}

func DefaultInstanceType(e environs.Environ) string {
	ecfg := e.(*environ).ecfg()
	return ecfg.defaultInstanceType()
}

// ImageDetails specify parameters used to start a test machine for the live tests.
type ImageDetails struct {
	Flavor  string
	ImageId string
}

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
