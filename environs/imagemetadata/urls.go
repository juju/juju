// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/juju/environs/storage"
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
	// If source is a raw directory, we need to append the file:// prefix
	// so it can be used as a URL.
	defaultURL := source
	u, err := url.Parse(source)
	if err != nil {
		return "", fmt.Errorf("invalid default image metadata URL %s: %v", defaultURL, err)
	}
	if u.Scheme == "" {
		defaultURL = "file://" + defaultURL
		if !strings.HasSuffix(defaultURL, "/"+storage.BaseImagesPath) {
			defaultURL = fmt.Sprintf("%s/%s", defaultURL, storage.BaseImagesPath)
		}
	}
	return defaultURL, nil
}
