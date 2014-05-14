// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"fmt"
	"net/url"
	"strings"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/utils"
)

// SupportsCustomSources represents an environment that
// can host tools metadata at provider specific sources.
type SupportsCustomSources interface {
	GetToolsSources() ([]simplestreams.DataSource, error)
}

// GetMetadataSources returns the sources to use when looking for
// simplestreams tools metadata. If env implements SupportsCustomSurces,
// the sources returned from that method will also be considered.
// The sources are configured to not use retries.
func GetMetadataSources(env environs.ConfigGetter) ([]simplestreams.DataSource, error) {
	return GetMetadataSourcesWithRetries(env, false)
}

// GetMetadataSourcesWithRetries returns the sources to use when looking for
// simplestreams tools metadata. If env implements SupportsCustomSurces,
// the sources returned from that method will also be considered.
// The sources are configured to use retries according to the value of allowRetry.
func GetMetadataSourcesWithRetries(env environs.ConfigGetter, allowRetry bool) ([]simplestreams.DataSource, error) {
	var sources []simplestreams.DataSource
	config := env.Config()
	if userURL, ok := config.ToolsURL(); ok {
		verify := utils.VerifySSLHostnames
		if !config.SSLHostnameVerification() {
			verify = utils.NoVerifySSLHostnames
		}
		sources = append(sources, simplestreams.NewURLDataSource("tools-metadata-url", userURL, verify))
	}
	if custom, ok := env.(SupportsCustomSources); ok {
		customSources, err := custom.GetToolsSources()
		if err != nil {
			return nil, err
		}
		sources = append(sources, customSources...)
	}

	defaultURL, err := ToolsURL(DefaultBaseURL)
	if err != nil {
		return nil, err
	}
	if defaultURL != "" {
		sources = append(sources,
			simplestreams.NewURLDataSource("default simplestreams", defaultURL, utils.VerifySSLHostnames))
	}
	for _, source := range sources {
		source.SetAllowRetry(allowRetry)
	}
	return sources, nil
}

// ToolsURL returns a valid tools URL constructed from source.
// source may be a directory, or a URL like file://foo or http://foo.
func ToolsURL(source string) (string, error) {
	if source == "" {
		return "", nil
	}
	// If source is a raw directory, we need to append the file:// prefix
	// so it can be used as a URL.
	defaultURL := source
	u, err := url.Parse(source)
	if err != nil {
		return "", fmt.Errorf("invalid default tools URL %s: %v", defaultURL, err)
	}
	if u.Scheme == "" {
		defaultURL = "file://" + defaultURL
		if !strings.HasSuffix(defaultURL, "/"+storage.BaseToolsPath) {
			defaultURL = fmt.Sprintf("%s/%s", defaultURL, storage.BaseToolsPath)
		}
	}
	return defaultURL, nil
}
