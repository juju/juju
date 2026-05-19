// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/cloud_mock.go github.com/juju/juju/apiserver/facades/client/cloud CredentialService,CloudService,CloudAccessService
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/credential_mock.go github.com/juju/juju/domain/credential/service CredentialValidator

var (
	CloudToParams = cloudToParams
)
