package openstack

import (
	"fmt"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/swift"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
	"net/http"
)

func init() {
}

var origMetadataHost = metadataHost

var metadataContent = `{"uuid": "d8e02d56-2648-49a3-bf97-6be8f1204f38",` +
	`"availability_zone": "nova", "hostname": "test.novalocal", ` +
	`"launch_index": 0, "meta": {"priority": "low", "role": "webserver"}, ` +
	`"public_keys": {"mykey": "ssh-rsa fake-key\n"}, "name": "test"}`

var metadataTestingBase = []jujutest.FileContent{
	{"/latest/meta-data/instance-id", "i-000abc"},
	{"/latest/meta-data/local-ipv4", "203.1.1.2"},
	{"/latest/meta-data/public-ipv4", "10.1.1.2"},
	{"/latest/openstack/2012-08-10/meta_data.json", metadataContent},
}

func UseTestMetadata(local bool) {
	if local {
		vfs := jujutest.NewVFS(metadataTestingBase)
		http.DefaultTransport.(*http.Transport).RegisterProtocol("file", http.NewFileTransport(vfs))
		metadataHost = "file:"
	} else {
		metadataHost = origMetadataHost
	}
}

var origMetadataJSON = metadataJSON

func UseMetadataJSON(path string) {
	if path != "" {
		metadataJSON = path
	} else {
		metadataJSON = origMetadataJSON
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

func FindInstanceSpec(e environs.Environ, series, arch, flavor string) (imageId, flavorId string, err error) {
	env := e.(*environ)
	spec, err := findInstanceSpec(env, &instanceConstraint{
		series: series,
		arch:   arch,
		region: env.ecfg().region(),
		flavor: flavor,
	})
	if err == nil {
		imageId = spec.imageId
		flavorId = spec.flavorId
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
