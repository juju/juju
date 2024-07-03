// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modelconfig/service/testing"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type ModelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

type serviceSuite struct {
	jtesting.IsolationSuite

	mockCtrlState *MockControllerState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockCtrlState = NewMockControllerState(ctrl)
	return ctrl
}

func (f ModelDefaultsProviderFunc) ModelDefaults(
	c context.Context,
) (modeldefaults.Defaults, error) {
	return f(c)
}

func (s *serviceSuite) TestSetModelConfig(c *gc.C) {
	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	var defaults ModelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{
			"foo": modeldefaults.DefaultAttributeValue{
				Source: config.JujuControllerSource,
				Value:  "bar",
			},
		}, nil
	}

	attrs := map[string]any{
		"name": "wallyworld",
		"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type": "sometype",
	}

	st := testing.NewState()
	defer st.Close()

	svc := NewWatchableService(defaults, config.ModelValidator(), nil, st, st)

	watcher, err := svc.Watch()
	c.Assert(err, jc.ErrorIsNil)
	var changes []string
	select {
	case changes = <-watcher.Changes():
	case <-ctx.Done():
		c.Fatal(ctx.Err())
	}
	c.Assert(len(changes), gc.Equals, 0)

	err = svc.SetModelConfig(ctx, attrs)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := svc.ModelConfig(ctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cfg.AllAttrs(), jc.DeepEquals, map[string]any{
		"agent-version":  jujuversion.Current.String(),
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"foo":            "bar",
		"secret-backend": "auto",
		"logging-config": "<root>=INFO",
	})

	select {
	case changes = <-watcher.Changes():
	case <-ctx.Done():
		c.Fatal(ctx.Err())
	}
	c.Check(changes, jc.SameContents, []string{
		"name", "uuid", "type", "foo", "secret-backend", "logging-config",
	})
}

func (s *serviceSuite) TestGetModelSecretBackend(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(nil, nil, s.mockCtrlState, nil)
	modelUUID := coremodel.UUID(jujutesting.ModelTag.Id())
	s.mockCtrlState.EXPECT().GetModelSecretBackend(gomock.Any(), modelUUID).Return("backend-id", nil)

	backendID, err := svc.GetModelSecretBackend(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(backendID, gc.Equals, "backend-id")
}

func (s *serviceSuite) TestSetModelSecretBackend(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(nil, nil, s.mockCtrlState, nil)
	modelUUID := coremodel.UUID(jujutesting.ModelTag.Id())
	s.mockCtrlState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "backend-id").Return(nil)
	err := svc.SetModelSecretBackend(context.Background(), modelUUID, "backend-id")
	c.Assert(err, jc.ErrorIsNil)
}
