// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmresources

import (
	"fmt"

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
	path := params.PathName
	if params.Series != "" {
		path = fmt.Sprintf("s/%s/%s", params.Series, path)
	}
	if params.Stream != "" {
		path = fmt.Sprintf("c/%s/%s", params.Stream, path)
	}
	if params.User != "" {
		path = fmt.Sprintf("u/%s/%s", params.User, path)
	}
	if params.Org != "" {
		path = fmt.Sprintf("org/%s/%s", params.Org, path)
	}
	if params.Revision != "" {
		path = fmt.Sprintf("%s/%s", path, params.Revision)
	}
	path = fmt.Sprintf("/%s/%s", params.Type, path)
	return path, nil
}

func validatePathParameters(params ResourceAttributes) error {
	if params.PathName == "" {
		return errors.New("resource path name cannot be empty")
	}
	if params.User != "" && params.Org != "" {
		return errors.New("both user and org cannot be specified together")
	}
	return nil
}
