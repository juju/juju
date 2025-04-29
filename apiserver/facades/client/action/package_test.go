// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/leadership"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/action State,Model,ApplicationService,ModelInfoService
//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination state_mock_test.go github.com/juju/juju/state Action,ActionReceiver
//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination leader_mock_test.go github.com/juju/juju/core/leadership Reader
//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination blockservices_mock_test.go github.com/juju/juju/apiserver/common BlockCommandService

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

type MockBaseSuite struct {
	State               *MockState
	Authorizer          *facademocks.MockAuthorizer
	ActionReceiver      *MockActionReceiver
	Leadership          *MockReader
	BlockCommandService *MockBlockCommandService
	ApplicationService  *MockApplicationService
	ModelInfoService    *MockModelInfoService
}

func (s *MockBaseSuite) NewActionAPI(c *gc.C) *ActionAPI {
	modelUUID := modeltesting.GenModelUUID(c)
	api, err := newActionAPI(s.State, nil, s.Authorizer, LeaderFactory(s.Leadership), s.ApplicationService, s.BlockCommandService, s.ModelInfoService, modelUUID)
	c.Assert(err, jc.ErrorIsNil)

	return api
}

func NewActionAPI(
	st *state.State,
	resources facade.Resources, authorizer facade.Authorizer, leadership leadership.Reader,
	applicationService ApplicationService,
	blockCommandService common.BlockCommandService,
	modelInfoService ModelInfoService,
	modelUUID coremodel.UUID,
) (*ActionAPI, error) {
	return newActionAPI(&stateShim{st: st}, resources, authorizer, LeaderFactory(leadership), applicationService, blockCommandService, modelInfoService, modelUUID)
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
