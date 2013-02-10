package openstack

import (
	"fmt"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/swift"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/trivial"
	"net/http"
)

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
