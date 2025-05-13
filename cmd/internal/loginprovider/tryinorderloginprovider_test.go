// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package loginprovider_test

import (
	"context"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/internal/loginprovider"
	internallogger "github.com/juju/juju/internal/logger"
)

type tryInOrderLoginProviderSuite struct{}

var _ = tc.Suite(&tryInOrderLoginProviderSuite{})

func (s *tryInOrderLoginProviderSuite) TestInOrderLoginProvider(c *tc.C) {
	p1 := &mockLoginProvider{err: errors.New("provider 1 error")}
	p2 := &mockLoginProvider{err: errors.New("provider 2 error")}
	header := http.Header{}
	header.Add("test", "foo")
	p3 := &mockLoginProvider{header: header, token: "successful-login-token"}

	logger := internallogger.GetLogger("juju.cmd.loginprovider")
	lp := loginprovider.NewTryInOrderLoginProvider(logger, p1, p2)
	_, err := lp.Login(context.Background(), nil)
	c.Assert(err, tc.ErrorMatches, "provider 2 error")

	lp = loginprovider.NewTryInOrderLoginProvider(logger, p1, p2, p3)
	_, err = lp.AuthHeader()
	c.Check(err, tc.ErrorMatches, api.ErrorLoginFirst.Error())
	_, err = lp.Login(context.Background(), nil)
	c.Check(err, tc.ErrorIsNil)
	got, err := lp.AuthHeader()
	c.Check(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, header)
}

type mockLoginProvider struct {
	err    error
	token  string
	header http.Header
}

func (p *mockLoginProvider) AuthHeader() (http.Header, error) {
	return p.header, nil
}

func (p *mockLoginProvider) Login(ctx context.Context, caller base.APICaller) (*api.LoginResultParams, error) {
	return &api.LoginResultParams{}, p.err
}
