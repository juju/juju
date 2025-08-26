// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// OfferURL represents the location of an offered application and its
// associated exported endpoints.
type OfferURL struct {
	// Source represents where the offer is hosted.
	// If empty, the model is another model in the same controller.
	Source string // "<controller-name>" or "<jaas>" or ""

	// ModelQualifier disambiguates the name of the model hosting the offer.
	// Older clients may set this to a username so we just deal with it as
	// a string and let the target controller which needs to use it to
	// resolve the offer figure out how to interpret it.
	ModelQualifier string

	// ModelName is the name of the model providing the exported endpoints.
	// It is only used for local URLs or for specifying models in the same
	// controller.
	ModelName string

	// Name is the name of the offer. This defaults to the name of the
	// application providing the exported endpoints if not specified by
	// when the offer is created.
	Name string
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
	path = fmt.Sprintf("%s.%s", path, u.Name)
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
	return strings.Contains(u.Name, ":")
}

// modelApplicationRegexp parses urls of the form source:qualifier/model.offername[:relname]
var modelApplicationRegexp = regexp.MustCompile(`(/?((?P<qualifier>[^/]+)/)?(?P<model>[^.]*)(\.(?P<offername>[^:]*(:.*)?))?)?`)

// ParseOfferURL parses the specified URL string into an OfferURL.
// The URL string is of one of the forms:
//
//	<model-name>.<offer-name>
//	<model-name>.<offer-name>:<relation-name>
//	<qualifier>/<model-name>.<offer-name>
//	<qualifier>/<model-name>.<offer-name>:<relation-name>
//	<source>:<model-name>.<offer-name>
//	<source>:<model-name>.<offer-name>:<relation-name>
//	<source>:<qualifier>/<model-name>.<offer-name>
//	<source>:<qualifier>/<model-name>.<offer-name>:<relation-name>
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
		result.Name = modelApplicationRegexp.ReplaceAllString(urlParts, "$offername")
	}
	if !valid || strings.Contains(result.ModelName, "/") || strings.Contains(result.Name, "/") {
		return nil, errors.Errorf("offer URL has invalid form, must be [<source>:][<qualifier>]<model>.<offername>: %q", urlStr)
	}
	if !allowIncomplete && result.ModelName == "" {
		return nil, errors.Errorf("offer URL is missing model")
	}
	if !allowIncomplete && result.Name == "" {
		return nil, errors.Errorf("offer URL is missing the name")
	}

	// Offer name part may contain a relation name part, so strip that bit out
	// before validating the name.
	offerName := strings.Split(result.Name, ":")[0]
	// Validate the resulting URL part values.
	// The qualifier part may come from older clients which use a username.
	// This is no longer a reasonable qualifier check, so we don't perform
	// any validation checks here; the target controller which handles the
	// URL will do any validation.
	if result.ModelName != "" && !names.IsValidModelName(result.ModelName) {
		return nil, errors.Errorf("model name %q %w", result.ModelName, coreerrors.NotValid)
	}
	// An offer name has the same requirements as an application name.
	if offerName != "" && !names.IsValidApplication(offerName) {
		return nil, errors.Errorf("offer name %q %w", offerName, coreerrors.NotValid)
	}
	return &result, nil
}

// MakeURL constructs an offer URL from the specified components.
func MakeURL(user, model, name, source string) string {
	base := fmt.Sprintf("%s/%s.%s", user, model, name)
	if source == "" {
		return base
	}
	return fmt.Sprintf("%s:%s", source, base)
}
