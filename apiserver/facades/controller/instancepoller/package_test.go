// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/juju/state"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package instancepoller_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/instancepoller ControllerConfigService,NetworkService,MachineService

type Patcher interface {
	PatchValue(ptr, value interface{})
}

func PatchState(p Patcher, st StateInterface) {
	p.PatchValue(&getState, func(*state.State, *state.Model) StateInterface {
		return st
	})
}
