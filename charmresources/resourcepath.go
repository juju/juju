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
	// TODO(wallyworld) - this is not complete, just enough to allow ResourceManager tests to work.
	path := params.PathName
	if path == "" {
		return "", errors.New("path cannot be empty")
	}
	if params.Series != "" {
		path = fmt.Sprintf("/s/%s/%s", params.Series, path)
	}
	return path, nil
}
