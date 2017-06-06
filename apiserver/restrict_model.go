// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"

	"github.com/juju/errors"
)

func modelFacadesOnly(facadeName, _ string) error {
	if !IsModelFacade(facadeName) {
		return errors.NewNotSupported(nil, fmt.Sprintf("facade %q not supported for model API connection", facadeName))
	}
	return nil
}

// IsModelFacade reports whether the given facade name can be accessed
// using a model connection.
func IsModelFacade(facadeName string) bool {
	return !controllerFacadeNames.Contains(facadeName)
}
