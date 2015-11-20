// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	gourl "net/url"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/set"
)

// ServiceURL represents the location of an offered service and its
// associated exported endpoints.
type ServiceURL struct {
	// Directory represents where the offer is hosted.
	Directory string // "local" or "<vendor>"

	// User is the user whose namespace in which the offer is made.
	User string

	// ServiceName is the name of the service providing the exported endpoints.
	ServiceName string
}

func (u *ServiceURL) path() string {
	var parts []string
	if u.User != "" {
		parts = append(parts, "u", u.User)
	}
	parts = append(parts, u.ServiceName)
	return strings.Join(parts, "/")
}

func (u *ServiceURL) String() string {
	return fmt.Sprintf("%s:/%s", u.Directory, u.path())
}

var supportedURLDirectories = []string{
	// TODO(wallyworld): just support local for now.
	"local", // for services hosted by a local service directory
}

// ParseServiceURL parses the specified URL string into a ServiceURL.
// The URL string is of one of the forms:
//  local:/u/<user>/<servicename>
//  <vendor>:/u/<user>/<servicename>
func ParseServiceURL(urlStr string) (*ServiceURL, error) {
	url, err := gourl.Parse(urlStr)
	if err != nil {
		return nil, errors.Errorf("cannot parse service URL: %q", urlStr)
	}
	if url.RawQuery != "" || url.Fragment != "" || url.User != nil {
		return nil, errors.Errorf("service URL %q has unrecognized parts", urlStr)
	}

	var result ServiceURL
	if url.Scheme == "" {
		url.Scheme = "local"
	}
	if url.Scheme != "" {
		result.Directory = url.Scheme
		if supported := IsSupportedURLDirectory(result.Directory); !supported {
			return nil, errors.Errorf("service URL has invalid directory: %q", urlStr)
		}
	}
	urlPath := strings.Trim(url.Path, "/")
	parts := strings.Split(urlPath, "/")
	if len(parts) < 1 || len(parts) > 3 {
		return nil, fmt.Errorf("service URL has invalid form: %q", urlStr)
	}

	// User
	if parts[0] != "u" {
		return nil, fmt.Errorf("service URL has invalid form, missing %q: %q", "/u/<user>", urlStr)
	}
	if !names.IsValidUser(parts[1]) {
		return nil, errors.NotValidf("user name %q", parts[1])
	}
	result.User = parts[1]

	// Service name
	if len(parts) < 3 {
		return nil, fmt.Errorf("service URL has invalid form, missing service name: %q", urlStr)
	}
	if !names.IsValidService(parts[2]) {
		return nil, errors.NotValidf("service name %q", parts[2])
	}
	result.ServiceName = parts[2]
	return &result, nil
}

// ServiceDirectoryForURL returns a service directory name, used to look up services,
// based on the specified URL.
func ServiceDirectoryForURL(urlStr string) (string, error) {
	url, err := ParseServiceURL(urlStr)
	if err != nil {
		return "", err
	}
	return url.Directory, nil
}

// IsSupportedURLDirectory determines if supplied URL directory name is supported.
var IsSupportedURLDirectory = func(dir string) bool {
	supportedSet := set.NewStrings(supportedURLDirectories...)
	return supportedSet.Contains(dir)
}
