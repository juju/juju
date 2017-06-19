// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

// The anonymousFacadeNames are the root names that can be accessed
// using an anonymous login. Any facade added here needs to perform
// its own authentication and authorisation if required.
var anonymousFacadeNames = set.NewStrings(
	"CrossModelRelations",
)

func anonymousFacadesOnly(facadeName, _ string) error {
	if !IsAnonymousFacade(facadeName) {
		return errors.NewNotSupported(nil, fmt.Sprintf("facade %q not supported for anonymous API connections", facadeName))
	}
	return nil
}

// IsAnonymousFacade reports whether the given facade name can be accessed
// using an anonymous connection.
func IsAnonymousFacade(facadeName string) bool {
	return anonymousFacadeNames.Contains(facadeName)
}
