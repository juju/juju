// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	gourl "net/url"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
)

// ApplicationURL represents the location of an offered application and its
// associated exported endpoints.
type ApplicationURL struct {
	// Directory represents where the offer is hosted.
	// If empty, the model is another model in the same controller.
	Directory string // "local" or "<vendor>" or ""

	// User is the user whose namespace in which the offer is made.
	User string

	// ModelName is the name of the model providing the exported endpoints.
	// It is only used for local URLs or for specifying models in the same
	// controller.
	ModelName string

	// ApplicationName is the name of the application providing the exported endpoints.
	ApplicationName string
}

// Path returns the path component of the URL.
func (u *ApplicationURL) Path() string {
	if u.Directory == "" {
		model := u.ModelName
		if u.User != "" {
			model = fmt.Sprintf("%s/%s", u.User, u.ModelName)
		}
		return fmt.Sprintf("%s.%s", model, u.ApplicationName)
	}
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
	if u.Directory == "" {
		return u.Path()
	}
	return fmt.Sprintf("%s:/%s", u.Directory, u.Path())
}

func (u *ApplicationURL) HasEndpoint() bool {
	return strings.Contains(u.ApplicationName, ":")
}

// modelApplicationRegexp parses urls of the form user/model.application[:relname]
var modelApplicationRegexp = regexp.MustCompile(`((?P<user>[^/]*)/)?(?P<model>[^.^/]*)\.(?P<application>[^:]*(:.*)?)`)

// applicationURLRegexp parses urls of the form local:/u/user/application
var applicationURLRegexp = regexp.MustCompile(`(u/(?P<user>[^/]*)/?)?(?P<application>.*)`)

// ParseApplicationURL parses the specified URL string into a ApplicationURL.
// The URL string is of one of the forms:
//  <model-name>.<application-name>
//  <model-name>.<application-name>:<relation-name>
//  <user>/<model-name>.<application-name>
//  <user>/<model-name>.<application-name>:<relation-name>
//  local:/u/<user>/<application-name>
//  local:/u/<user>/<model-name>/<application-name>
//  <vendor>:/u/<user>/<application-name>
func ParseApplicationURL(urlStr string) (*ApplicationURL, error) {
	urlParts, err := parseApplicationURLParts(urlStr, false)
	if err != nil {
		return nil, err
	}
	if urlParts.ModelName == "" && urlParts.Directory == "" {
		urlParts.Directory = "local"
	}
	url := ApplicationURL(*urlParts)
	return &url, nil
}

// ApplicationURLParts contains various attributes of a URL.
type ApplicationURLParts ApplicationURL

// ParseApplicationURLParts parses a partial URL, filling out what parts are supplied.
// This method is used to generate a filter used to query matching application URLs.
func ParseApplicationURLParts(urlStr string) (*ApplicationURLParts, error) {
	return parseApplicationURLParts(urlStr, true)
}

func parseApplicationURLParts(urlStr string, allowIncomplete bool) (*ApplicationURLParts, error) {
	var result ApplicationURLParts
	if modelApplicationRegexp.MatchString(urlStr) {
		result.User = modelApplicationRegexp.ReplaceAllString(urlStr, "$user")
		result.ModelName = modelApplicationRegexp.ReplaceAllString(urlStr, "$model")
		result.ApplicationName = modelApplicationRegexp.ReplaceAllString(urlStr, "$application")
	} else {
		url, err := gourl.Parse(urlStr)
		if err != nil {
			return nil, errors.Errorf("cannot parse application URL: %q", urlStr)
		}
		if url.RawQuery != "" || url.Fragment != "" || url.User != nil {
			return nil, errors.Errorf("application URL %q has unrecognized parts", urlStr)
		}

		urlPath := strings.Trim(url.Path, "/")
		if applicationURLRegexp.MatchString(urlPath) {
			result.Directory = url.Scheme
			result.User = applicationURLRegexp.ReplaceAllString(urlPath, "$user")
			result.ApplicationName = applicationURLRegexp.ReplaceAllString(urlPath, "$application")
			if result.User == "" && !allowIncomplete {
				return nil, errors.Errorf("application URL has invalid form, missing %q: %q", "/u/<user>", urlStr)
			}
			if result.ApplicationName == "" && !allowIncomplete {
				return nil, errors.Errorf("application URL has invalid form, missing application name: %q", urlStr)
			}
		} else {
			return nil, errors.Errorf("application URL has invalid form: %q", urlStr)
		}
		if strings.Index(result.ApplicationName, "/") > 0 {
			return nil, errors.Errorf("application URL has too many parts: %q", urlStr)
		}
	}
	// Validate the resulting URL part values.
	if result.User != "" && !names.IsValidUser(result.User) {
		return nil, errors.NotValidf("user name %q", result.User)
	}
	if result.ModelName != "" && !names.IsValidModelName(result.ModelName) {
		return nil, errors.NotValidf("model name %q", result.ModelName)
	}
	// Application name part may contain a relation name part, so strip that bit out
	// before validating the name.
	appName := strings.Split(result.ApplicationName, ":")[0]
	if appName != "" && !names.IsValidApplication(appName) {
		return nil, errors.NotValidf("application name %q", appName)
	}
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
