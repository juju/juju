// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/remoterelations_mocks.go github.com/juju/juju/apiserver/facades/controller/remoterelations RemoteRelationsState,ControllerConfigAPI,ExternalControllerService,SecretService

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
