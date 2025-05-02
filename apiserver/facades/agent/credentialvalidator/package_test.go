// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen  -typed -package mocks -destination mocks/services_mock.go github.com/juju/juju/apiserver/facades/agent/credentialvalidator ModelService,ModelInfoService,CredentialService,CloudService
//go:generate go run go.uber.org/mock/mockgen  -typed -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/core/watcher NotifyWatcher

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
