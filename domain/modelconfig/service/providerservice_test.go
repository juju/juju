// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type providerServiceSuite struct {
	mockState *MockProviderState
}

var _ = gc.Suite(&providerServiceSuite{})

func (s *providerServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockState = NewMockProviderState(ctrl)
	return ctrl
}

func (s *providerServiceSuite) TestModelConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name": "wallyworld",
			"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type": "sometype",
		},
		nil,
	)

	svc := NewProviderService(s.mockState)
	cfg, err := svc.ModelConfig(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(cfg.AllAttrs(), jc.DeepEquals, map[string]any{
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"logging-config": "<root>=INFO",
	})
}
