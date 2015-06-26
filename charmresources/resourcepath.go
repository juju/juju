// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmresources

import (
	"path"

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

func validatePathParameters(params ResourceAttributes) error {
	if params.PathName == "" {
		return errors.New("resource path name cannot be empty")
	}
	if params.User != "" && params.Org != "" {
		return errors.New("both user and org cannot be specified together")
	}
	return nil
}
