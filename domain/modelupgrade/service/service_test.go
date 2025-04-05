// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/modelupgrade"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type serviceSuite struct {
	testing.IsolationSuite

	state      *MockState
	prechecker *MockJujuUpgradePrechecker

	service *ProviderService
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.service = NewProviderService(s.state, func(ctx context.Context) (JujuUpgradePrechecker, error) {
		if s.prechecker != nil {
			return s.prechecker, nil
		}
		return nil, errors.Errorf("provider %w", coreerrors.NotSupported)
	}, loggertesting.WrapCheckLog(c))
	return ctrl
}

func (s *serviceSuite) TestPerformProviderChecks(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.prechecker = NewMockJujuUpgradePrechecker(ctrl)
	s.prechecker.EXPECT().PreparePrechecker(gomock.Any())
	s.prechecker.EXPECT().PrecheckUpgradeOperations()

	s.state.EXPECT().GetModelVersionInfo(gomock.Any()).Return(semversion.MustParse("6.6.5"), true, nil)

	err := s.service.PerformProviderChecks(context.Background(), modelupgrade.UpgradeModelParams{
		TargetVersion: semversion.MustParse("6.6.7"),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestPerformProviderChecksNonControllerModel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.prechecker = NewMockJujuUpgradePrechecker(ctrl)

	s.state.EXPECT().GetModelVersionInfo(gomock.Any()).Return(semversion.MustParse("6.6.5"), false, nil)

	err := s.service.PerformProviderChecks(context.Background(), modelupgrade.UpgradeModelParams{
		TargetVersion: semversion.Zero,
	})
	c.Assert(err, jc.ErrorIsNil)
}
