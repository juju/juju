// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"testing"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/state"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/action State,Model,ApplicationService
//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination state_mock_test.go github.com/juju/juju/state Action,ActionReceiver
//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination leader_mock_test.go github.com/juju/juju/core/leadership Reader
//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination blockservices_mock_test.go github.com/juju/juju/apiserver/common BlockCommandService

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type MockBaseSuite struct {
	State               *MockState
	Authorizer          *facademocks.MockAuthorizer
	ActionReceiver      *MockActionReceiver
	Leadership          *MockReader
	BlockCommandService *MockBlockCommandService
	ApplicationService  *MockApplicationService
}

func (s *MockBaseSuite) NewActionAPI(c *gc.C) *ActionAPI {
	api, err := newActionAPI(s.State, nil, s.Authorizer, LeaderFactory(s.Leadership), s.ApplicationService, s.BlockCommandService)
	c.Assert(err, jc.ErrorIsNil)

	api.tagToActionReceiverFn = s.tagToActionReceiverFn
	return api
}

func (s *MockBaseSuite) tagToActionReceiverFn(
	func(names.Tag) (state.Entity, error),
) func(tag string) (state.ActionReceiver, error) {
	return func(tag string) (state.ActionReceiver, error) { return s.ActionReceiver, nil }
}

func NewActionAPI(
	st *state.State,
	resources facade.Resources, authorizer facade.Authorizer, leadership leadership.Reader,
	applicationService ApplicationService,
	blockCommandService common.BlockCommandService,
) (*ActionAPI, error) {
	return newActionAPI(&stateShim{st: st}, resources, authorizer, LeaderFactory(leadership), applicationService, blockCommandService)
}

type FakeLeadership struct {
	AppLeaders map[string]string
}

func (l FakeLeadership) Leaders() (map[string]string, error) {
	return l.AppLeaders, nil
}

func LeaderFactory(reader leadership.Reader) func() (leadership.Reader, error) {
	return func() (leadership.Reader, error) { return reader, nil }
}
