// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package loginprovider_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/internal/loginprovider"
)

type tryInOrderLoginProviderSuite struct{}

var _ = gc.Suite(&tryInOrderLoginProviderSuite{})

func (s *tryInOrderLoginProviderSuite) Test(c *gc.C) {
	p1 := &mockLoginProvider{err: errors.New("provider 1 error")}
	p2 := &mockLoginProvider{err: errors.New("provider 2 error")}
	p3 := &mockLoginProvider{}

	logger := loggo.GetLogger("juju.cmd.loginprovider")
	lp := loginprovider.NewTryInOrderLoginProvider(logger, p1, p2)
	_, err := lp.Login(context.Background(), nil)
	c.Assert(err, gc.ErrorMatches, "provider 2 error")

	lp = loginprovider.NewTryInOrderLoginProvider(logger, p1, p2, p3)
	_, err = lp.Login(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
}

type mockLoginProvider struct {
	err error
}

func (p *mockLoginProvider) Login(ctx context.Context, caller base.APICaller) (*api.LoginResultParams, error) {
	return &api.LoginResultParams{}, p.err
}
