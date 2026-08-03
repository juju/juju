// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package traceservices

import (
	"context"

	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	tracingservice "github.com/juju/juju/domain/tracing/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type baseSuite struct {
	domaintesting.ControllerSuite

	logger       logger.Logger
	dbGetter     changestream.WatchableDBGetter
	traceService *stubTraceService
}

func (s *baseSuite) setupMocks(c *tc.C) {
	s.logger = loggertesting.WrapCheckLog(c)
	s.dbGetter = stubDBGetter{}
	s.traceService = &stubTraceService{}
}

type stubDBGetter struct {
	changestream.WatchableDBGetter
}

type stubTraceService struct{}

func (s *stubTraceService) Tracing() *tracingservice.WatchableService {
	return nil
}

var _ tracingservice.State = (*stubTracingState)(nil)

type stubTracingState struct{}

func (s *stubTracingState) SetCharmTracingConfig(context.Context, map[string]string, []string) error {
	return nil
}

func (s *stubTracingState) GetCharmTracingConfig(context.Context) (map[string]string, error) {
	return nil, nil
}

func (s *stubTracingState) SetWorkloadTracingConfig(context.Context, map[string]string, []string) error {
	return nil
}

func (s *stubTracingState) GetWorkloadTracingConfig(context.Context) (map[string]string, error) {
	return nil, nil
}

func (s *stubTracingState) InitialWatchStatementForWorkloadTracingConfig() (string, string) {
	return "", ""
}

func (s *stubTracingState) NamespaceForWatchWorkloadTracingConfig() string {
	return ""
}
