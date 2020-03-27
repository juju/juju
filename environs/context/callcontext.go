// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

// ModelCredentialInvalidator defines a point of use interface for invalidating
// a model credential.
type ModelCredentialInvalidator interface {

	// InvalidateModelCredential invalidate cloud credential for the model.
	InvalidateModelCredential(string) error
}

// CallContext creates a CloudCallContext for use when calling environ methods
// that may require invalidate a cloud credential.
func CallContext(ctx ModelCredentialInvalidator) *CloudCallContext {
	callCtx := NewCloudCallContext()
	callCtx.InvalidateCredentialFunc = ctx.InvalidateModelCredential
	return callCtx
}
