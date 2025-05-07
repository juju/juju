// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"
)

type providerServiceSuite struct {
	mockState *MockProviderState
}

var _ = tc.Suite(&providerServiceSuite{})

func (s *providerServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockState = NewMockProviderState(ctrl)
	return ctrl
}

func (s *providerServiceSuite) TestModelConfig(c *tc.C) {
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
	c.Check(err, tc.ErrorIsNil)
	c.Check(cfg.AllAttrs(), tc.DeepEquals, map[string]any{
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"logging-config": "<root>=INFO",
	})
}
