// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	modeltesting "github.com/juju/juju/core/model/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
)

type ModelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

type serviceSuite struct {
	mockState *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (f ModelDefaultsProviderFunc) ModelDefaults(
	c context.Context,
) (modeldefaults.Defaults, error) {
	return f(c)
}

func noopDefaultsProvider() ModelDefaultsProvider {
	return ModelDefaultsProviderFunc(func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{}, nil
	})
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockState = NewMockState(ctrl)
	return ctrl
}

// TestGetModelConfigContainsAgentInformation checks that the models agent
// version and stream gets injected into the model config.
func (s *serviceSuite) TestGetModelConfigContainsAgentInformation(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			config.NameKey: "foo",
			config.UUIDKey: modelUUID.String(),
			config.TypeKey: "type",
		}, nil,
	)
	s.mockState.EXPECT().GetModelAgentVersionAndStream(gomock.Any()).Return(
		jujuversion.Current.String(), coreagentbinary.AgentStreamReleased.String(), nil,
	)

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), s.mockState)
	cfg, err := svc.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg.AgentStream(), gc.Equals, coreagentbinary.AgentStreamReleased.String())
}

// TestUpdateModelConfigAgentStream checks that the model agent stream value
// cannot be changed via model config and if it is we get back an errors that
// satisfies [config.ValidationError].
func (s *serviceSuite) TestUpdateModelConfigAgentStream(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name": "wallyworld",
			"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type": "sometype",
		}, nil,
	)
	s.mockState.EXPECT().GetModelAgentVersionAndStream(gomock.Any()).Return("1.2.3", "released", nil)

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), s.mockState)
	err := svc.UpdateModelConfig(
		context.Background(),
		map[string]any{
			"agent-stream": "proposed",
		},
		nil,
	)

	val, is := errors.AsType[*config.ValidationError](err)
	c.Check(is, jc.IsTrue)
	c.Check(val.InvalidAttrs, gc.DeepEquals, []string{"agent-stream"})
}

// TestUpdateModelConfigNoAgentStreamChange checks that the model agent stream
// does not change in model config results in no error and the value is removed
// from model config before persiting.
func (s *serviceSuite) TestUpdateModelConfigNoAgentStreamChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name": "wallyworld",
			"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type": "sometype",
		}, nil,
	)
	s.mockState.EXPECT().GetModelAgentVersionAndStream(gomock.Any()).Return("1.2.3", "released", nil)
	s.mockState.EXPECT().UpdateModelConfig(
		gomock.Any(),
		map[string]string{},
		gomock.Any(),
	)

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), s.mockState)
	err := svc.UpdateModelConfig(
		context.Background(),
		map[string]any{
			"agent-stream": "released",
		},
		nil,
	)

	c.Assert(err, jc.ErrorIsNil)
}

//func (s *serviceSuite) TestSetModelConfig(c *gc.C) {
//	defer s.setupMocks(c).Finish()
//	ctx, cancel := jujutesting.LongWaitContext()
//	defer cancel()
//
//	var defaults ModelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
//		return modeldefaults.Defaults{
//			"foo": modeldefaults.DefaultAttributeValue{
//				Controller: "bar",
//			},
//		}, nil
//	}
//
//	attrs := map[string]any{
//		"name": "wallyworld",
//		"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
//		"type": "sometype",
//	}
//
//	st := testing.NewState()
//	defer st.Close()
//
//	svc := NewWatchableService(defaults, config.ModelValidator(), st, st)
//
//	watcher, err := svc.Watch()
//	c.Assert(err, jc.ErrorIsNil)
//	var changes []string
//	select {
//	case changes = <-watcher.Changes():
//	case <-ctx.Done():
//		c.Fatal(ctx.Err())
//	}
//	c.Assert(len(changes), gc.Equals, 0)
//
//	err = svc.SetModelConfig(ctx, attrs)
//	c.Assert(err, jc.ErrorIsNil)
//
//	cfg, err := svc.ModelConfig(ctx)
//	c.Assert(err, jc.ErrorIsNil)
//
//	c.Check(cfg.AllAttrs(), jc.DeepEquals, map[string]any{
//		"agent-version":  jujuversion.Current.String(),
//		"name":           "wallyworld",
//		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
//		"type":           "sometype",
//		"foo":            "bar",
//		"logging-config": "<root>=INFO",
//	})
//
//	select {
//	case changes = <-watcher.Changes():
//	case <-ctx.Done():
//		c.Fatal(ctx.Err())
//	}
//	c.Check(changes, jc.SameContents, []string{
//		"name", "uuid", "type", "foo", "logging-config",
//	})
//}
//
