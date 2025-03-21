// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	"golang.org/x/crypto/acme/autocert"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}

func (s *serviceSuite) TestCheckCacheMiss(c *gc.C) {
	defer s.setupMocks(c).Finish()

	certName := "test-cert-name"
	s.state.EXPECT().Get(gomock.Any(), certName).Return(nil, errors.Errorf("autocert %s: %w", certName, coreerrors.NotFound))

	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	certbytes, err := svc.Get(context.Background(), certName)
	c.Assert(certbytes, gc.IsNil)
	c.Assert(err, jc.ErrorIs, autocert.ErrCacheMiss)
}

func (s *serviceSuite) TestCheckAnyError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	certName := "test-cert-name"
	s.state.EXPECT().Get(gomock.Any(), certName).Return(nil, errors.New("state error"))

	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	certbytes, err := svc.Get(context.Background(), certName)
	c.Assert(certbytes, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "state error")
}
