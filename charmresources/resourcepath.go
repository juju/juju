// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmresources

import (
	"path"
	"regexp"
	"strings"

	"github.com/juju/errors"
)

// ResourcePath constructs a path to use in a resource store from
// the specified resource attributes.
func ResourcePath(params ResourceAttributes) (string, error) {
	if err := validatePathParameters(params); err != nil {
		return "", err
	}
	if params.Type == "" {
		params.Type = string(ResourceTypeBlob)
	}
	resPath := params.PathName
	if params.Series != "" {
		resPath = path.Join("s", params.Series, resPath)
	}
	if params.Stream != "" {
		resPath = path.Join("c", params.Stream, resPath)
	}
	if params.User != "" {
		resPath = path.Join("u", params.User, resPath)
	}
	if params.Org != "" {
		resPath = path.Join("org", params.Org, resPath)
	}
	if params.Revision != "" {
		resPath = path.Join(resPath, params.Revision)
	}
	resPath = path.Join("/"+params.Type, resPath)
	return resPath, nil
}

var segmentSnippet = "[^/]+"
var resourcePathRe = regexp.MustCompile(
	"^/" +
		"(" + segmentSnippet + ")/" + // type
		"(?:org/(" + segmentSnippet + ")/)?" + // org
		"(?:u/(" + segmentSnippet + ")/)?" + // user
		"(?:c/(" + segmentSnippet + ")/)?" + // stream
		"(?:s/(" + segmentSnippet + ")/)?" + // series
		"(" + segmentSnippet + ")" + // path-name
		"(?:/(" + segmentSnippet + "))?" + // revision
		"$",
)

// ParseResourcePath parses a resource path into its constituent
// ResourceAttributes.
func ParseResourcePath(resourcePath string) (ResourceAttributes, error) {
	var attrs ResourceAttributes
	submatch := resourcePathRe.FindStringSubmatch(resourcePath)
	if submatch == nil {
		return attrs, errors.Errorf("invalid resource path %q", resourcePath)
	}
	attrs.Type = submatch[1]
	attrs.Org = submatch[2]
	attrs.User = submatch[3]
	attrs.Stream = submatch[4]
	attrs.Series = submatch[5]
	attrs.PathName = submatch[6]
	attrs.Revision = submatch[7]
	if err := validatePathParameters(attrs); err != nil {
		return attrs, errors.Trace(err)
	}
	return attrs, nil
}

func validatePathParameters(params ResourceAttributes) error {
	if params.PathName == "" {
		return errors.New("resource path name cannot be empty")
	}
	if strings.Contains(params.PathName, "/") {
		return errors.New(`resource path name cannot contain "/"`)
	}
	if params.User != "" && params.Org != "" {
		return errors.New("both user and org cannot be specified together")
	}
	return nil
}
