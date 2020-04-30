// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"fmt"

	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/utils"
)

// ImageMetadataURL returns a valid image metadata URL constructed from source.
// source may be a directory, or a URL like file://foo or http://foo.
func ImageMetadataURL(source, stream string) (string, error) {
	if source == "" {
		return "", nil
	}
	// If the image metadata is coming from the official cloud images site,
	// set up the correct path according to the images stream requested.
	if source == UbuntuCloudImagesURL {
		cloudImagesPath := ReleasedImagesPath
		if stream != "" && stream != ReleasedStream {
			cloudImagesPath = stream
		}
		source = fmt.Sprintf("%s/%s", source, cloudImagesPath)
	}

	return utils.GetURL(source, storage.BaseImagesPath)
}
