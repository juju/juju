package openstack

import (
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
