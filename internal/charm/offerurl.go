// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/model"
)

// OfferURL represents the location of an offered application and its
// associated exported endpoints.
type OfferURL struct {
	// Source represents where the offer is hosted.
	// If empty, the model is another model in the same controller.
	Source string // "<controller-name>" or "<jaas>" or ""

	// ModelQualifier disambiguates the name of the model hosting the offer.
	ModelQualifier string

	// ModelName is the name of the model providing the exported endpoints.
	// It is only used for local URLs or for specifying models in the same
	// controller.
	ModelName string

	// ApplicationName is the name of the application providing the exported endpoints.
	ApplicationName string
}

// Path returns the path component of the URL.
func (u *OfferURL) Path() string {
	var parts []string
	if u.ModelQualifier != "" {
		parts = append(parts, u.ModelQualifier)
	}
	if u.ModelName != "" {
		parts = append(parts, u.ModelName)
	}
	path := strings.Join(parts, "/")
	path = fmt.Sprintf("%s.%s", path, u.ApplicationName)
	if u.Source == "" {
		return path
	}
	return fmt.Sprintf("%s:%s", u.Source, path)
}

func (u *OfferURL) String() string {
	return u.Path()
}

// AsLocal returns a copy of the URL with an empty (local) source.
func (u *OfferURL) AsLocal() *OfferURL {
	localURL := *u
	localURL.Source = ""
	return &localURL
}

// HasEndpoint returns whether this offer URL includes an
// endpoint name in the application name.
func (u *OfferURL) HasEndpoint() bool {
	return strings.Contains(u.ApplicationName, ":")
}

// modelApplicationRegexp parses urls of the form controller:qualifier/model.application[:relname]
var modelApplicationRegexp = regexp.MustCompile(`(/?((?P<qualifier>[^/]+)/)?(?P<model>[^.]*)(\.(?P<application>[^:]*(:.*)?))?)?`)

// IsValidOfferURL ensures that a URL string is a valid OfferURL.
func IsValidOfferURL(urlStr string) bool {
	_, err := ParseOfferURL(urlStr)
	return err == nil
}

// ParseOfferURL parses the specified URL string into an OfferURL.
// The URL string is of one of the forms:
//
//	<model-name>.<application-name>
//	<model-name>.<application-name>:<relation-name>
//	<qualifier>/<model-name>.<application-name>
//	<qualifier>/<model-name>.<application-name>:<relation-name>
//	<controller>:<qualifier>/<model-name>.<application-name>
//	<controller>:<qualifier>/<model-name>.<application-name>:<relation-name>
func ParseOfferURL(urlStr string) (*OfferURL, error) {
	return parseOfferURL(urlStr)
}

// parseOfferURL parses the specified URL string into an OfferURL.
func parseOfferURL(urlStr string) (*OfferURL, error) {
	urlParts, err := parseOfferURLParts(urlStr, false)
	if err != nil {
		return nil, err
	}
	url := OfferURL(*urlParts)
	return &url, nil
}

// OfferURLParts contains various attributes of a URL.
type OfferURLParts OfferURL

// ParseOfferURLParts parses a partial URL, filling out what parts are supplied.
// This method is used to generate a filter used to query matching offer URLs.
func ParseOfferURLParts(urlStr string) (*OfferURLParts, error) {
	return parseOfferURLParts(urlStr, true)
}

var endpointRegexp = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

func maybeParseSource(urlStr string) (source, rest string) {
	parts := strings.Split(urlStr, ":")
	switch len(parts) {
	case 3:
		return parts[0], parts[1] + ":" + parts[2]
	case 2:
		if endpointRegexp.MatchString(parts[1]) {
			return "", urlStr
		}
		return parts[0], parts[1]
	}
	return "", urlStr
}

func parseOfferURLParts(urlStr string, allowIncomplete bool) (*OfferURLParts, error) {
	var result OfferURLParts
	source, urlParts := maybeParseSource(urlStr)

	valid := !strings.HasPrefix(urlStr, ":")
	valid = valid && modelApplicationRegexp.MatchString(urlParts)
	if valid {
		result.Source = source
		result.ModelQualifier = modelApplicationRegexp.ReplaceAllString(urlParts, "$qualifier")
		result.ModelName = modelApplicationRegexp.ReplaceAllString(urlParts, "$model")
		result.ApplicationName = modelApplicationRegexp.ReplaceAllString(urlParts, "$application")
	}
	if !valid || strings.Contains(result.ModelName, "/") || strings.Contains(result.ApplicationName, "/") {
		// TODO(wallyworld) - update error message when we support multi-controller and JAAS CMR
		return nil, errors.Errorf("application offer URL has invalid form, must be [<qualifier/]<model>.<appname>: %q", urlStr)
	}
	if !allowIncomplete && result.ModelName == "" {
		return nil, errors.Errorf("application offer URL is missing model")
	}
	if !allowIncomplete && result.ApplicationName == "" {
		return nil, errors.Errorf("application offer URL is missing application")
	}

	// Application name part may contain a relation name part, so strip that bit out
	// before validating the name.
	appName := strings.Split(result.ApplicationName, ":")[0]
	// Validate the resulting URL part values.
	if result.ModelQualifier != "" && !model.IsValidQualifier(result.ModelQualifier) {
		return nil, errors.NotValidf("qualifier %q", result.ModelQualifier)
	}
	if result.ModelName != "" && !names.IsValidModelName(result.ModelName) {
		return nil, errors.NotValidf("model name %q", result.ModelName)
	}
	if appName != "" && !names.IsValidApplication(appName) {
		return nil, errors.NotValidf("application name %q", appName)
	}
	return &result, nil
}

// MakeURL constructs an offer URL from the specified components.
func MakeURL(qualifier, model, application, controller string) string {
	base := fmt.Sprintf("%s/%s.%s", qualifier, model, application)
	if controller == "" {
		return base
	}
	return fmt.Sprintf("%s:%s", controller, base)
}
