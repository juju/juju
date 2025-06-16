// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"os"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/leadership"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/action ApplicationService,ModelInfoService
//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination leader_mock_test.go github.com/juju/juju/core/leadership Reader
//go:generate go run go.uber.org/mock/mockgen -typed -package action -destination blockservices_mock_test.go github.com/juju/juju/apiserver/common BlockCommandService

func TestMain(m *testing.M) {
	os.Exit(func() int {
		defer coretesting.MgoTestMain()()
		return m.Run()
	}())
}

type MockBaseSuite struct {
	Authorizer          *facademocks.MockAuthorizer
	Leadership          *MockReader
	BlockCommandService *MockBlockCommandService
	ApplicationService  *MockApplicationService
	ModelInfoService    *MockModelInfoService
}

func (s *MockBaseSuite) NewActionAPI(c *tc.C) *ActionAPI {
	modelUUID := modeltesting.GenModelUUID(c)
	api, err := newActionAPI(nil, s.Authorizer, LeaderFactory(s.Leadership), s.ApplicationService, s.BlockCommandService, s.ModelInfoService, modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	return api
}

func NewActionAPI(
	resources facade.Resources, authorizer facade.Authorizer, leadership leadership.Reader,
	applicationService ApplicationService,
	blockCommandService common.BlockCommandService,
	modelInfoService ModelInfoService,
	modelUUID coremodel.UUID,
) (*ActionAPI, error) {
	return newActionAPI(resources, authorizer, LeaderFactory(leadership), applicationService, blockCommandService, modelInfoService, modelUUID)
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
