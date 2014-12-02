// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/juju/utils"
)

func GetURL(source, basePath string) (string, error) {
	// If source is a raw directory, we need to append the file:// prefix
	// so it can be used as a URL.
	defaultURL := source
	u, err := url.Parse(source)
	if err != nil {
		return "", fmt.Errorf("invalid default %s URL %s: %v", basePath, defaultURL, err)
	}

	switch u.Scheme {
	case "http", "https", "file", "test":
		return source, nil
	}

	if filepath.IsAbs(defaultURL) {
		defaultURL = utils.MakeFileURL(defaultURL)
		if !strings.HasSuffix(defaultURL, "/"+basePath) {
			defaultURL = fmt.Sprintf("%s/%s", defaultURL, basePath)
		}
	} else {
		return "", fmt.Errorf("%s is not an absolute path", source)
	}
	return defaultURL, nil
}
