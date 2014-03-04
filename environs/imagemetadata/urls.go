// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"fmt"
	"net/url"
	"strings"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
)

// SupportsCustomSources represents an environment that
// can host image metadata at provider specific sources.
type SupportsCustomSources interface {
	GetImageSources() ([]simplestreams.DataSource, error)
}

// GetMetadataSources returns the sources to use when looking for
// simplestreams image id metadata for the given stream. If env implements
// SupportsCustomSources, the sources returned from that method will also
// be considered.
func GetMetadataSources(env environs.ConfigGetter) ([]simplestreams.DataSource, error) {
	var sources []simplestreams.DataSource
	config := env.Config()
	if userURL, ok := config.ImageMetadataURL(); ok {
		verify := simplestreams.VerifySSLHostnames
		if !config.SSLHostnameVerification() {
			verify = simplestreams.NoVerifySSLHostnames
		}
		sources = append(sources, simplestreams.NewURLDataSource("image-metadata-url", userURL, verify))
	}
	if custom, ok := env.(SupportsCustomSources); ok {
		customSources, err := custom.GetImageSources()
		if err != nil {
			return nil, err
		}
		sources = append(sources, customSources...)
	}

	defaultURL, err := ImageMetadataURL(DefaultBaseURL, config.ImageStream())
	if err != nil {
		return nil, err
	}
	if defaultURL != "" {
		sources = append(sources, simplestreams.NewURLDataSource("default cloud images", defaultURL, simplestreams.VerifySSLHostnames))
	}
	return sources, nil
}

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
