// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
)

type tryInOrderLoginProviderSuite struct{}

var _ = gc.Suite(&tryInOrderLoginProviderSuite{})

func (s *tryInOrderLoginProviderSuite) Test(c *gc.C) {
	p1 := &mockLoginProvider{err: errors.New("provider 1 error")}
	p2 := &mockLoginProvider{err: errors.New("provider 2 error")}
	p3 := &mockLoginProvider{}

	lp := api.NewTryInOrderLoginProvider(p1, p2)
	_, err := lp.Login(context.Background(), nil)
	c.Assert(err, gc.ErrorMatches, "provider 2 error")

	lp = api.NewTryInOrderLoginProvider(p1, p2, p3)
	_, err = lp.Login(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
}

type mockLoginProvider struct {
	err error
}

func (p *mockLoginProvider) Login(ctx context.Context, caller base.APICaller) (*api.LoginResultParams, error) {
	return &api.LoginResultParams{}, p.err
}
