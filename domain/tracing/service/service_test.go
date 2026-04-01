// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	st *MockState
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestSetCharmTracingConfigAllFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := CharmTracingConfig{
		HTTPEndpoint:  "http://localhost:4318",
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "cert-data",
	}

	s.st.EXPECT().SetCharmTracingConfig(gomock.Any(), map[string]string{
		httpEndpointKey:  "http://localhost:4318",
		grpcEndpointKey:  "localhost:4317",
		caCertificateKey: "cert-data",
	}, []string{}).Return(nil)

	err := NewService(s.st).SetCharmTracingConfig(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetCharmTracingConfigNoFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().SetCharmTracingConfig(gomock.Any(), map[string]string{}, []string{
		httpEndpointKey,
		grpcEndpointKey,
		caCertificateKey,
	}).Return(nil)

	err := NewService(s.st).SetCharmTracingConfig(c.Context(), CharmTracingConfig{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetCharmTracingConfigPartialFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := CharmTracingConfig{
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "cert-data",
	}

	s.st.EXPECT().SetCharmTracingConfig(gomock.Any(), map[string]string{
		grpcEndpointKey:  "localhost:4317",
		caCertificateKey: "cert-data",
	}, []string{
		httpEndpointKey,
	}).Return(nil)

	err := NewService(s.st).SetCharmTracingConfig(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetCharmTracingConfigStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().SetCharmTracingConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		errors.Errorf("boom"),
	)

	err := NewService(s.st).SetCharmTracingConfig(c.Context(), CharmTracingConfig{})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestGetCharmTracingConfigAllFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetCharmTracingConfig(gomock.Any()).Return(map[string]string{
		httpEndpointKey:  "http://localhost:4318",
		grpcEndpointKey:  "localhost:4317",
		caCertificateKey: "cert-data",
	}, nil)

	config, err := NewService(s.st).GetCharmTracingConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, CharmTracingConfig{
		HTTPEndpoint:  "http://localhost:4318",
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "cert-data",
	})
}

func (s *serviceSuite) TestGetCharmTracingConfigPartialFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetCharmTracingConfig(gomock.Any()).Return(map[string]string{
		grpcEndpointKey: "localhost:4317",
	}, nil)

	config, err := NewService(s.st).GetCharmTracingConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config, tc.DeepEquals, CharmTracingConfig{
		HTTPEndpoint:  "",
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "",
	})
}

func (s *serviceSuite) TestGetCharmTracingConfigStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetCharmTracingConfig(gomock.Any()).Return(nil, errors.Errorf("boom"))

	_, err := NewService(s.st).GetCharmTracingConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)

	c.Cleanup(func() { s.st = nil })

	return ctrl
}
