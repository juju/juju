// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errorutils

import (
	"github.com/juju/errors"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
)

// ServiceError returns the *azure.ServiceError underlying the
// supplied error, if any, and a bool indicating whether one
// was found.
func ServiceError(err error) (*azure.ServiceError, bool) {
	err = errors.Cause(err)
	if d, ok := err.(autorest.DetailedError); ok {
		err = d.Original
	}
	if r, ok := err.(*azure.RequestError); ok {
		return r.ServiceError, true
	}
	return nil, false
}
