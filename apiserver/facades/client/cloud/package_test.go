// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/cloud_mock.go github.com/juju/juju/apiserver/facades/client/cloud CredentialService,CloudService,CloudAccessService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/credential_mock.go github.com/juju/juju/domain/credential/service CredentialValidator

var (
	CloudToParams = cloudToParams
)
