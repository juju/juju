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

func (s *serviceSuite) TestSetTracingConfigAllFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := TracingConfig{
		HTTPEndpoint:  "http://localhost:4318",
		HTTPSEndpoint: "https://localhost:4318",
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "cert-data",
	}

	s.st.EXPECT().SetTracingConfig(gomock.Any(), map[string]string{
		httpEndpointKey:  "http://localhost:4318",
		httpsEndpointKey: "https://localhost:4318",
		grpcEndpointKey:  "localhost:4317",
		caCertificateKey: "cert-data",
	}, []string{}).Return(nil)

	err := NewService(s.st).SetTracingConfig(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetTracingConfigNoFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().SetTracingConfig(gomock.Any(), map[string]string{}, []string{
		httpEndpointKey,
		httpsEndpointKey,
		grpcEndpointKey,
		caCertificateKey,
	}).Return(nil)

	err := NewService(s.st).SetTracingConfig(c.Context(), TracingConfig{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetTracingConfigPartialFields(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := TracingConfig{
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "cert-data",
	}

	s.st.EXPECT().SetTracingConfig(gomock.Any(), map[string]string{
		grpcEndpointKey:  "localhost:4317",
		caCertificateKey: "cert-data",
	}, []string{
		httpEndpointKey,
		httpsEndpointKey,
	}).Return(nil)

	err := NewService(s.st).SetTracingConfig(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetTracingConfigStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().SetTracingConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		errors.Errorf("boom"),
	)

	err := NewService(s.st).SetTracingConfig(c.Context(), TracingConfig{})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)

	c.Cleanup(func() { s.st = nil })

	return ctrl
}
