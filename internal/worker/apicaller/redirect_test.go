// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"context"
	"errors"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/crossmodel"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/apicaller"
)

var (
	errBoom     = errors.New("boom")
	srcAddr     = "1.2.3.4:17070"
	dstAddr     = "7.7.7.7:1234"
	modelUUID   = "89cbf64e-3708-495f-871f-e58a34c41b33"
	redirectURI = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
)

type redirectSuite struct {
	testhelpers.IsolationSuite
}

func TestRedirectSuite(t *testing.T) {
	tc.Run(t, &redirectSuite{})
}

// stubConnection is a non-nil connection value; nothing calls methods on
// the connections returned by the helpers under test.
type stubConnection struct {
	api.Connection
}

// recordingUpdater implements apicaller.ExternalControllerUpdater, recording
// the controller info it was passed.
type recordingUpdater struct {
	updated   bool
	info      crossmodel.ControllerInfo
	returnErr error
}

func (u *recordingUpdater) UpdateExternalController(_ context.Context, info crossmodel.ControllerInfo) error {
	u.updated = true
	u.info = info
	return u.returnErr
}

func newRedirectError() *api.RedirectError {
	return &api.RedirectError{
		Servers: []network.MachineHostPorts{
			network.NewMachineHostPorts(1234, "7.7.7.7"),
		},
		CACert:          "redirected-ca-cert",
		ControllerTag:   names.NewControllerTag(redirectURI),
		ControllerAlias: "redirected-alias",
	}
}

func (s *redirectSuite) TestNoRedirect(c *tc.C) {
	expectConn := stubConnection{}
	var opens int
	open := func(_ context.Context, info *api.Info) (api.Connection, error) {
		opens++
		return expectConn, nil
	}

	apiInfo := &api.Info{Addrs: []string{srcAddr}, CACert: "src-ca-cert"}
	conn, redirect, err := apicaller.NewExternalControllerConnectionWithRedirect(c.Context(), apiInfo, open)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn == expectConn, tc.IsTrue)
	c.Check(redirect, tc.IsNil)
	c.Check(opens, tc.Equals, 1)
	c.Check(apiInfo.Addrs, tc.DeepEquals, []string{srcAddr})
	c.Check(apiInfo.CACert, tc.Equals, "src-ca-cert")
}

func (s *redirectSuite) TestFollowsRedirect(c *tc.C) {
	redirectErr := newRedirectError()
	expectConn := stubConnection{}
	var opens int
	open := func(_ context.Context, info *api.Info) (api.Connection, error) {
		defer func() { opens++ }()
		if opens == 0 {
			return nil, redirectErr
		}
		// The retry must see the redirected addresses and CA cert, and the
		// original tag and model tag untouched.
		c.Check(info.Addrs, tc.DeepEquals, []string{dstAddr})
		c.Check(info.CACert, tc.Equals, "redirected-ca-cert")
		return expectConn, nil
	}

	apiInfo := &api.Info{Addrs: []string{srcAddr}, CACert: "src-ca-cert"}
	conn, redirect, err := apicaller.NewExternalControllerConnectionWithRedirect(c.Context(), apiInfo, open)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn == expectConn, tc.IsTrue)
	c.Check(redirect == redirectErr, tc.IsTrue)
	c.Check(opens, tc.Equals, 2)
	c.Check(apiInfo.Addrs, tc.DeepEquals, []string{dstAddr})
	c.Check(apiInfo.CACert, tc.Equals, "redirected-ca-cert")
}

func (s *redirectSuite) TestRedirectSecondOpenFails(c *tc.C) {
	redirectErr := newRedirectError()
	var opens int
	open := func(_ context.Context, info *api.Info) (api.Connection, error) {
		defer func() { opens++ }()
		if opens == 0 {
			return nil, redirectErr
		}
		return nil, errBoom
	}

	conn, redirect, err := apicaller.NewExternalControllerConnectionWithRedirect(c.Context(), &api.Info{}, open)
	c.Assert(err, tc.ErrorIs, errBoom)
	c.Check(conn, tc.IsNil)
	c.Check(redirect, tc.IsNil)
	c.Check(opens, tc.Equals, 2)
}

func (s *redirectSuite) TestNonRedirectError(c *tc.C) {
	var opens int
	open := func(_ context.Context, info *api.Info) (api.Connection, error) {
		opens++
		return nil, errBoom
	}

	conn, redirect, err := apicaller.NewExternalControllerConnectionWithRedirect(c.Context(), &api.Info{}, open)
	c.Assert(err, tc.ErrorIs, errBoom)
	c.Check(conn, tc.IsNil)
	c.Check(redirect, tc.IsNil)
	c.Check(opens, tc.Equals, 1)
}

func (s *redirectSuite) TestSaveMigratedModelController(c *tc.C) {
	updater := &recordingUpdater{}
	apiInfo := &api.Info{Addrs: []string{dstAddr}, CACert: "redirected-ca-cert"}

	err := apicaller.SaveMigratedModelController(c.Context(), updater, newRedirectError(), apiInfo, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(updater.updated, tc.IsTrue)
	c.Check(updater.info, tc.DeepEquals, crossmodel.ControllerInfo{
		ControllerUUID: redirectURI,
		Alias:          "redirected-alias",
		Addrs:          []string{dstAddr},
		CACert:         "redirected-ca-cert",
		ModelUUIDs:     []string{modelUUID},
	})
}

func (s *redirectSuite) TestSaveMigratedModelControllerInvalidControllerTag(c *tc.C) {
	updater := &recordingUpdater{}
	redirectErr := &api.RedirectError{
		Servers: []network.MachineHostPorts{
			network.NewMachineHostPorts(1234, "7.7.7.7"),
		},
		CACert: "redirected-ca-cert",
	}
	apiInfo := &api.Info{Addrs: []string{dstAddr}, CACert: "redirected-ca-cert"}

	err := apicaller.SaveMigratedModelController(c.Context(), updater, redirectErr, apiInfo, modelUUID)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(updater.updated, tc.IsFalse)
}

func (s *redirectSuite) TestSaveMigratedModelControllerUpdaterError(c *tc.C) {
	updater := &recordingUpdater{returnErr: errBoom}
	apiInfo := &api.Info{Addrs: []string{dstAddr}, CACert: "redirected-ca-cert"}

	err := apicaller.SaveMigratedModelController(c.Context(), updater, newRedirectError(), apiInfo, modelUUID)
	c.Assert(err, tc.ErrorIs, errBoom)
	c.Check(updater.updated, tc.IsTrue)
}
