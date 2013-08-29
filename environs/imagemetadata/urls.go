// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import "launchpad.net/juju-core/environs"

// SupportsCustomURLs instances can host image metadata at provider specific URLs.
type SupportsCustomURLs interface {
	GetImageBaseURLs() ([]string, error)
}

// GetMetadataURLs returns the URLs to use when looking for simplestreams image id metadata.
func GetMetadataURLs(cloud environs.HasConfig) ([]string, error) {
	var urls []string
	if userURL, ok := cloud.Config().ImageMetadataURL(); ok {
		urls = append(urls, userURL)
	}
	if custom, ok := cloud.(SupportsCustomURLs); ok {
		customURLs, err := custom.GetImageBaseURLs()
		if err != nil {
			return nil, err
		}
		urls = append(urls, customURLs...)
	}

	if DefaultBaseURL != "" {
		urls = append(urls, DefaultBaseURL)
	}
	return urls, nil
}
