// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	gourl "net/url"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
)

// ApplicationURL represents the location of an offered application and its
// associated exported endpoints.
type ApplicationURL struct {
	// Directory represents where the offer is hosted.
	Directory string // "local" or "<vendor>"

	// User is the user whose namespace in which the offer is made.
	User string

	// ModelName is the name of the model providing the exported endpoints.
	// It is only used for local URLs.
	ModelName string

	// ApplicationName is the name of the application providing the exported endpoints.
	ApplicationName string
}

// Path returns the path component of the URL.
func (u *ApplicationURL) Path() string {
	var parts []string
	if u.User != "" {
		parts = append(parts, "u", u.User)
	}
	if u.Directory == "local" && u.ModelName != "" {
		parts = append(parts, u.ModelName)
	}
	parts = append(parts, u.ApplicationName)
	return strings.Join(parts, "/")
}

func (u *ApplicationURL) String() string {
	return fmt.Sprintf("%s:/%s", u.Directory, u.Path())
}

var supportedURLDirectories = []string{
	// TODO(wallyworld): just support local for now.
	"local", // for applications hosted by a local application directory
}

// ParseApplicationURL parses the specified URL string into a ApplicationURL.
// The URL string is of one of the forms:
//  local:/u/<user>/<applicationname>
//  local:/u/<user>/<envname>/<applicationname>
//  <vendor>:/u/<user>/<applicationname>
func ParseApplicationURL(urlStr string) (*ApplicationURL, error) {
	url, err := gourl.Parse(urlStr)
	if err != nil {
		return nil, errors.Errorf("cannot parse application URL: %q", urlStr)
	}
	if url.RawQuery != "" || url.Fragment != "" || url.User != nil {
		return nil, errors.Errorf("application URL %q has unrecognized parts", urlStr)
	}

	var result ApplicationURL
	if url.Scheme == "" {
		url.Scheme = "local"
	}
	if url.Scheme != "" {
		result.Directory = url.Scheme
	}
	urlPath := strings.Trim(url.Path, "/")
	parts := strings.Split(urlPath, "/")
	if len(parts) < 1 || len(parts) > 4 {
		return nil, fmt.Errorf("application URL has invalid form: %q", urlStr)
	}

	// User
	if parts[0] != "u" {
		return nil, fmt.Errorf("application URL has invalid form, missing %q: %q", "/u/<user>", urlStr)
	}
	if !names.IsValidUser(parts[1]) {
		return nil, errors.NotValidf("user name %q", parts[1])
	}
	result.User = parts[1]

	// Application name
	if len(parts) < 3 {
		return nil, fmt.Errorf("application URL has invalid form, missing application name: %q", urlStr)
	}

	// Figure out what URL parts we have.
	envPart := -1
	applicationPart := 2
	if len(parts) == 4 {
		envPart = 2
		applicationPart = 3
	}

	if envPart > 0 {
		result.ModelName = parts[envPart]
	}

	if !names.IsValidApplication(parts[applicationPart]) {
		return nil, errors.NotValidf("application name %q", parts[applicationPart])
	}
	result.ApplicationName = parts[applicationPart]
	return &result, nil
}

// ApplicationDirectoryForURL returns a application directory name, used to look up applications,
// based on the specified URL.
func ApplicationDirectoryForURL(urlStr string) (string, error) {
	url, err := ParseApplicationURL(urlStr)
	if err != nil {
		return "", err
	}
	return url.Directory, nil
}

// ApplicationURLParts contains various attributes of a URL.
type ApplicationURLParts ApplicationURL

// ParseApplicationURLParts parses a partial URL, filling out what parts are supplied.
// TODO(wallyworld) update ParseApplicationURL to use this method and perform additional validation on top.
func ParseApplicationURLParts(urlStr string) (*ApplicationURLParts, error) {
	url, err := gourl.Parse(urlStr)
	if err != nil {
		return nil, errors.Errorf("cannot parse application URL: %q", urlStr)
	}
	if url.RawQuery != "" || url.Fragment != "" || url.User != nil {
		return nil, errors.Errorf("application URL %q has unrecognized parts", urlStr)
	}

	var result ApplicationURLParts
	if url.Scheme != "" {
		result.Directory = url.Scheme
	}
	urlPath := strings.Trim(url.Path, "/")
	parts := strings.Split(urlPath, "/")

	if len(parts) < 2 {
		switch len(parts) {
		case 1:
			result.ApplicationName = parts[0]
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
			return nil, fmt.Errorf("application URL has too many parts: %q", urlStr)
		}
		result.ModelName = parts[0]
		result.ApplicationName = parts[1]
		return &result, nil
	}

	if len(parts) == 2 {
		return &result, nil
	}

	// Figure out what other URL parts we have.
	envPart := -1
	applicationPart := 2
	if len(parts) == 4 {
		envPart = 2
		applicationPart = 3
	}

	if envPart > 0 {
		result.ModelName = parts[envPart]
	}

	if !names.IsValidApplication(parts[applicationPart]) {
		return nil, errors.NotValidf("application name %q", parts[applicationPart])
	}
	result.ApplicationName = parts[applicationPart]
	return &result, nil
}
