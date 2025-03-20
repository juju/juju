// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details

package common

import (
	"github.com/juju/collections/set"
	ociCommon "github.com/oracle/oci-go-sdk/v65/common"
)

// Oracle bundles authorisation errors into HTTP 401, HTTP 404 and
// HTTP 409, without specifically indicating that it's an authorisation
// failure. Each response includes a Code field that we can match against,
// but they remain intentionally ambiguous.
//
// The codes we match against are:
//   - HTTP 401 ("NotAuthenticated")
//   - HTTP 404 ("NotAuthorizedOrNotFound")
//   - HTTP 409 ("NotAuthorizedOrResourceAlreadyExists")
//
// As we're not generating any API calls manually, it's unlikely
// that we'll be striking URIs that don't exist at all, therefore we assume
// auth issues are causing the errors.
//
// For more details, see https://docs.cloud.oracle.com/iaas/Content/API/References/apierrors.htm
var authErrorCodes = set.NewStrings(
	"NotAuthenticated",
	"NotAuthorizedOrResourceAlreadyExists",
	"NotAuthorizedOrNotFound",
)

// IsAuthorisationFailure reports whether the error is related to
// attempting to access the provider with invalid or expired credentials.
func IsAuthorisationFailure(err error) bool {
	if err == nil {
		return false
	}

	serviceError, ok := err.(ociCommon.ServiceError)
	if !ok {
		// Just to double check, also try the SDK's
		// implementation. This isn't checked first, because
		// it is hard to test.
		serviceError, ok = ociCommon.IsServiceError(err)
	}

	if ok && authErrorCodes.Contains(serviceError.GetCode()) {
		return true
	}

	return false
}
