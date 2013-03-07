package openstack

import (
	"bytes"
	"fmt"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/swift"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
	"net/http"
	"os"
)


type VirtualFile struct {
	bytes.Reader
}

var _ http.File = (*VirtualFile)(nil)

func (f *VirtualFile) Close() error {
	return nil
}

func (f *VirtualFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, nil
}

func (f *VirtualFile) Stat() (os.FileInfo, error) {
	return nil, fmt.Errorf("Can't stat VirtualFile")
}

func init() {
	http.DefaultTransport.(*http.Transport).RegisterProtocol("file", http.NewFileTransport(http.Dir("testdata")))
}

var origMetadataHost = metadataHost

func UseTestMetadata(local bool) {
	if local {
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
