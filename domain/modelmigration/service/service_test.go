// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/providertracker"
)

type serviceSuite struct {
	provider *MockProvider
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.provider = NewMockProvider(ctrl)
	return ctrl
}

func (s *serviceSuite) providerGetter(_ *gc.C) providertracker.ProviderGetter[Provider] {
	return func(_ context.Context) (Provider, error) {
		return s.provider, nil
	}
}

// TestMachinesFromProviderDiscrepancy is testing the return value from
// [Service.CheckMachines] and that it reports discrepancies from the cloud.
// TODO (tlm): This test is not fully implemented and will be done when instance
// data is moved over to DQlite.
func (s *serviceSuite) TestMachinesFromProviderDiscrepancy(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().AllInstances(gomock.Any()).Return(nil, nil)

	_, err := New(s.providerGetter(c)).CheckMachines(context.Background())
	c.Check(err, jc.ErrorIsNil)
}
