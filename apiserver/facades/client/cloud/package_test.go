// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/cloud_mock.go github.com/juju/juju/apiserver/facades/client/cloud ModelCredentialService,CredentialService,CloudService,CloudAccessService
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/credential_mock.go github.com/juju/juju/domain/credential/service CredentialValidator

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

var (
	CloudToParams = cloudToParams
)
