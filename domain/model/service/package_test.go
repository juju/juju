// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/model/service ControllerState,EnvironVersionProvider,ModelDeleter,ModelState,State,ModelResourcesProvider,CloudInfoProvider,WatcherFactory,RegionProvider

type statusHistoryGetter struct {
	loggerContextGetter logger.LoggerContextGetter
	clock               clock.Clock
}

func newStatusHistoryGetter(c *tc.C) StatusHistoryGetter {
	return statusHistoryGetter{
		loggerContextGetter: loggertesting.WrapCheckLogForContextGetter(c),
		clock:               clock.WallClock,
	}
}

// GetLoggerContext returns a logger context for the given model UUID.
func (l statusHistoryGetter) GetStatusHistoryForModel(ctx context.Context, modelUUID coremodel.UUID) (StatusHistory, error) {
	loggerContext, err := l.loggerContextGetter.GetLoggerContext(ctx, modelUUID)
	if err != nil {
		return nil, err
	}

	logger := loggerContext.GetLogger("juju.services")
	return domain.NewStatusHistory(logger, l.clock), nil
}
