// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems

import (
	"github.com/juju/errors"
)

type enabledChecker interface {
	IsEnabled(name string) (bool, error)
}

func EnsureEnabled(name string, is enabledChecker) error {
	enabled, err := is.IsEnabled(name)
	if err != nil {
		return errors.Trace(err)
	}
	if !enabled {
		return errors.NotFoundf("service %q", name)
	}
	return nil
}
