// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	gourl "net/url"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
)

// ServiceURL represents the location of an offered service and its
// associated exported endpoints.
type ServiceURL struct {
	// Directory represents where the offer is hosted.
	Directory string // "local" or "<vendor>"

	// User is the user whose namespace in which the offer is made.
	User string

	// EnvironmentName is the name of the environment providing the exported endpoints.
	// It is only used for local URLs.
	EnvironmentName string

	// ServiceName is the name of the service providing the exported endpoints.
	ServiceName string
}

// Path returns the path component of the URL.
func (u *ServiceURL) Path() string {
	var parts []string
	if u.User != "" {
		parts = append(parts, "u", u.User)
	}
	if u.Directory == "local" && u.EnvironmentName != "" {
		parts = append(parts, u.EnvironmentName)
	}
	parts = append(parts, u.ServiceName)
	return strings.Join(parts, "/")
}

func (u *ServiceURL) String() string {
	return fmt.Sprintf("%s:/%s", u.Directory, u.Path())
}

var supportedURLDirectories = []string{
	// TODO(wallyworld): just support local for now.
	"local", // for services hosted by a local service directory
}

// ParseServiceURL parses the specified URL string into a ServiceURL.
// The URL string is of one of the forms:
//  local:/u/<user>/<servicename>
//  local:/u/<user>/<envname>/<servicename>
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
	}
	urlPath := strings.Trim(url.Path, "/")
	parts := strings.Split(urlPath, "/")
	if len(parts) < 1 || len(parts) > 4 {
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

	// Figure out what URL parts we have.
	envPart := -1
	servicePart := 2
	if len(parts) == 4 {
		envPart = 2
		servicePart = 3
	}

	if envPart > 0 {
		result.EnvironmentName = parts[envPart]
	}

	if !names.IsValidService(parts[servicePart]) {
		return nil, errors.NotValidf("service name %q", parts[servicePart])
	}
	result.ServiceName = parts[servicePart]
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

// ServiceURLParts contains various attributes of a URL.
type ServiceURLParts ServiceURL

// ParseServiceURLParts parses a partial URL, filling out what parts are supplied.
// TODO(wallyworld) update ParseServiceURL to use this method and perform additional validation on top.
func ParseServiceURLParts(urlStr string) (*ServiceURLParts, error) {
	url, err := gourl.Parse(urlStr)
	if err != nil {
		return nil, errors.Errorf("cannot parse service URL: %q", urlStr)
	}
	if url.RawQuery != "" || url.Fragment != "" || url.User != nil {
		return nil, errors.Errorf("service URL %q has unrecognized parts", urlStr)
	}

	var result ServiceURLParts
	if url.Scheme != "" {
		result.Directory = url.Scheme
	}
	urlPath := strings.Trim(url.Path, "/")
	parts := strings.Split(urlPath, "/")

	if len(parts) < 2 {
		switch len(parts) {
		case 1:
			result.ServiceName = parts[0]
		}
		return &result, nil
	}

	if parts[0] == "u" {
		if !names.IsValidUser(parts[1]) {
			return nil, errors.NotValidf("user name %q", parts[1])
		}
		result.User = parts[1]
	} else {
		if len(parts) > 2 {
			return nil, fmt.Errorf("service URL has too many parts: %q", urlStr)
		}
		result.EnvironmentName = parts[0]
		result.ServiceName = parts[1]
		return &result, nil
	}

	if len(parts) == 2 {
		return &result, nil
	}

	// Figure out what other URL parts we have.
	envPart := -1
	servicePart := 2
	if len(parts) == 4 {
		envPart = 2
		servicePart = 3
	}

	if envPart > 0 {
		result.EnvironmentName = parts[envPart]
	}

	if !names.IsValidService(parts[servicePart]) {
		return nil, errors.NotValidf("service name %q", parts[servicePart])
	}
	result.ServiceName = parts[servicePart]
	return &result, nil
}
