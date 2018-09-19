// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import "github.com/juju/juju/apiserver/facade"

var (
	InstanceTypes                     = instanceTypes
	ValidateNewCredentialForModelFunc = &validateNewCredentialForModelFunc
)

func NewCloudTestingAPI(backend, ctlrBackend Backend, authorizer facade.Authorizer) *CloudAPI {
	return &CloudAPI{
		backend:     backend,
		ctlrBackend: ctlrBackend,
		authorizer:  authorizer,
	}
}
