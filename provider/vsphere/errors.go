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
func HandleCredentialError(err error, env *sessionEnviron, ctx context.ProviderCallContext) {
	if err == nil {
		return
	}
	// LP #1849194: fell into a situation where we can either have an invalid
	// credential OR user issued a VM spec that has no rights to, e.g. on a
	// Resource Pool that it has no permissions on using "zone" on add-machine.
	// To discover if the credentials are valid, run a command that MUST return
	// OK: find folder defined on vm-folder credentials
	_, errfind := env.client.FindFolder(env.ctx, env.getVMFolder())
	if errfind != nil {
		// This is a credential issue. Now, move to mark credentials as invalid
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
	}
}
