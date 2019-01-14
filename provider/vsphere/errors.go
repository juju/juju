// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"strings"

	"github.com/juju/errors"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
)

const (
	// serverFaultCode is the code on the SOAP fault for an
	// authentication error.
	serverFaultCode = "ServerFaultCode"

	// loginErrorFragment is what we look for in the message string to
	// determine whether this is a login failure. (Using a fragment
	// instead of the exact string to try to avoid breaking if a
	// 'cosmetic' is made to the message.)
	loginErrorFragment = "incorrect user name or password"
)

// IsAuthorisationFailure determines whether the given error indicates
// that the vsphere credential used is bad.
func IsAuthorisationFailure(err error) bool {
	baseErr := errors.Cause(err)
	if !soap.IsSoapFault(baseErr) {
		return false
	}
	fault := soap.ToSoapFault(baseErr)
	if fault.Code != serverFaultCode {
		return false
	}
	_, isPermissionError := fault.Detail.Fault.(types.NoPermission)
	if isPermissionError {
		return true
	}
	// Otherwise it could be a login error.
	return strings.Contains(fault.String, loginErrorFragment)
}

// HandleCredentialError marks the current credential as invalid if
// the passed vsphere error indicates it should be.
func HandleCredentialError(err error, ctx context.ProviderCallContext) {
	common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
}
