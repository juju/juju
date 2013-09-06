// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import "launchpad.net/juju-core/environs/config"

// SupportsCustomURLs instances can host tools metadata at provider specific URLs.
type SupportsCustomURLs interface {
	GetToolsBaseURLs() ([]string, error)
}

// GetMetadataURLs returns the URLs to use when looking for simplestreams tools metadata.
func GetMetadataURLs(cloudInst config.HasConfig) ([]string, error) {
	var urls []string
	if userURL, ok := cloudInst.Config().ToolsURL(); ok {
		urls = append(urls, userURL)
	}
	if custom, ok := cloudInst.(SupportsCustomURLs); ok {
		customURLs, err := custom.GetToolsBaseURLs()
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
