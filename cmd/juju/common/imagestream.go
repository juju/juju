package common

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
)

// ShouldImplyForce attempts to work out if it's a requirement for
// the operator to supply force.
func ShouldImplyForce(modelCfg map[string]interface{}) bool {
	imageStream, imageStreamFound := modelCfg[config.ImageStreamKey]
	imageMetadataURL, imageMetadataURLFound := modelCfg[config.ImageMetadataURLKey]

	// If we can't find the config image stream data with in the model, when we
	// expect to always have it in the model, then we don't have any idea what
	// is expected other than asking the operator to use --force.
	if !imageStreamFound || !imageMetadataURLFound {
		return false
	}

	// If the operator hasn't supplied the default image meta data url, we
	// expect that they don't require force. In the instance that the value we
	// get back from the model config isn't a string, then return asking for
	// force.
	if url, ok := imageMetadataURL.(string); !ok {
		return false
	} else if !imagemetadata.ValidDefaultImageMetadataURL(url) {
		return true
	}

	// If the operator is passing a non-release image-stream, then they should
	// be allowed to bootstrap without a force flag.
	stream, ok := imageStream.(string)
	if !ok {
		return false
	}
	return !imagemetadata.IsReleasedStream(stream)
}
