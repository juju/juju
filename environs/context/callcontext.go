// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import stdcontext "context"

// ModelCredentialInvalidator defines a point of use interface for invalidating
// a model credential.
type ModelCredentialInvalidator interface {
	// InvalidateModelCredential invalidate cloud credential for the model.
	InvalidateModelCredential(string) error
}

// CallContext creates a CloudCallContext for use when calling environ methods
// that may require invalidate a cloud credential.
func CallContext(credential ModelCredentialInvalidator) *CloudCallContext {
	// TODO(wallyworld) - pass in the stdcontext
	callCtx := NewCloudCallContext(stdcontext.Background())
	callCtx.InvalidateCredentialFunc = credential.InvalidateModelCredential
	return callCtx
}
