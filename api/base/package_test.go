// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/clientfacade_mock.go github.com/juju/juju/api/base APICallCloser,ClientFacade
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/caller_mock.go github.com/juju/juju/api/base APICaller,FacadeCaller

