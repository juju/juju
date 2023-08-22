// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/leadership"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

//go:generate go run go.uber.org/mock/mockgen -package uniter_test -destination lxdprofile_mock_test.go github.com/juju/juju/apiserver/facades/agent/uniter LXDProfileBackend,LXDProfileMachine,LXDProfileUnit
//go:generate go run go.uber.org/mock/mockgen -package uniter_test -destination newlxdprofile_mock_test.go github.com/juju/juju/apiserver/facades/agent/uniter LXDProfileBackendV2,LXDProfileMachineV2,LXDProfileUnitV2,LXDProfileCharmV2,LXDProfileModelV2
//go:generate go run go.uber.org/mock/mockgen -package uniter_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/agent/uniter ControllerConfigGetter

func ptr[T any](v T) *T {
	return &v
}

type fakeBroker struct {
	caas.Broker
}

func (*fakeBroker) APIVersion() (string, error) {
	return "6.66", nil
}

type fakeToken struct {
	err error
}

func (t *fakeToken) Check() error {
	return t.err
}

type fakeLeadershipChecker struct {
	isLeader bool
}

type token struct {
	isLeader          bool
	unit, application string
}

func (t *token) Check() error {
	if !t.isLeader {
		return leadership.NewNotLeaderError(t.unit, t.application)
	}
	return nil
}

func (f *fakeLeadershipChecker) LeadershipCheck(applicationName, unitName string) leadership.Token {
	return &token{isLeader: f.isLeader, unit: unitName, application: applicationName}
}
