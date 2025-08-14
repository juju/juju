// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coremodel "github.com/juju/juju/core/model"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
)

type ModelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

type serviceSuite struct {
	mockState *MockState
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

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

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockState = NewMockState(ctrl)
	return ctrl
}

// TestGetModelConfigContainsAgentInformation checks that the models agent
// version and stream gets injected into the model config.
func (s *serviceSuite) TestGetModelConfigContainsAgentInformation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := coremodel.GenUUID(c)
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
	cfg, err := svc.ModelConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.AgentStream(), tc.Equals, coreagentbinary.AgentStreamReleased.String())
}

// TestUpdateModelConfigAgentStream checks that the model agent stream value
// cannot be changed via model config and if it is we get back an errors that
// satisfies [config.ValidationError].
func (s *serviceSuite) TestUpdateModelConfigAgentStream(c *tc.C) {
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
		c.Context(),
		map[string]any{
			"agent-stream": "proposed",
		},
		nil,
	)

	val, is := errors.AsType[*config.ValidationError](err)
	c.Check(is, tc.IsTrue)
	c.Check(val.InvalidAttrs, tc.DeepEquals, []string{"agent-stream"})
}

// TestUpdateModelConfigNoAgentStreamChange checks that the model agent stream
// does not change in model config results in no error and the value is removed
// from model config before persiting.
func (s *serviceSuite) TestUpdateModelConfigNoAgentStreamChange(c *tc.C) {
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
		c.Context(),
		map[string]any{
			"agent-stream": "released",
		},
		nil,
	)

	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetModelConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var defaults ModelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{
			"foo": modeldefaults.DefaultAttributeValue{
				Controller: "bar",
			},
		}, nil
	}

	attrs := map[string]any{
		"name": "wallyworld",
		"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type": "sometype",
	}

	s.mockState.EXPECT().SetModelConfig(gomock.Any(), map[string]string{
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"foo":            "bar",
		"logging-config": "<root>=INFO",
	})

	svc := NewService(defaults, config.ModelValidator(), s.mockState)
	err := svc.SetModelConfig(c.Context(), attrs)
	c.Assert(err, tc.ErrorIsNil)
}
