// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/errors"
)

type notAvailable struct {
	errors.Err
}

// NotAvailable returns an error which satisfies IsNotAvailable.
func NotAvailable(thing string) error {
	return &notAvailable{
		errors.NewErr("%s is not available", thing),
	}
}

// IsNotAvailable reports whether err was creates with NotAvailable().
func IsNotAvailable(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*notAvailable)
	return ok
}
