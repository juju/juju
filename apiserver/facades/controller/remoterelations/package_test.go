// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/remoterelations_mocks.go github.com/juju/juju/apiserver/facades/controller/remoterelations RemoteRelationsState,ControllerConfigAPI,ExternalControllerService,SecretService

