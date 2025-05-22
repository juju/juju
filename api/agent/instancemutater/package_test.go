// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/caller_mock.go github.com/juju/juju/api/base APICaller,FacadeCaller
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/machinemutater_mock.go github.com/juju/juju/api/agent/instancemutater MutaterMachine

