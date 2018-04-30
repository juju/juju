// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
)

// CallContext is a placeholder for a provider call context that will provide useful
// callbacks and other functions. For example, there will be a callback to invalid cloud
// credential that a controller uses if provider will receive some errors
// that will indicate tht cloud considers that credential invalid.
// TODO (anastasiamac 2018-04-27) flesh it out.
type CallContext struct{}

// InvalidateCredentialCallback implements context.InvalidateCredentialCallback.
func (*CallContext) InvalidateCredentialCallback() error {
	return errors.NotImplementedf("InvalidateCredentialCallback")
}
