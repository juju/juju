// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient_test

import (
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/tools/lxdclient"
)

var (
	_ = gc.Suite(&utilsSuite{})
)

type utilsSuite struct {
	lxdclient.BaseSuite
}

func (s *utilsSuite) TestEnableHTTPSListener(c *gc.C) {
	var client mockConfigSetter
	err := lxdclient.EnableHTTPSListener(&client)
	c.Assert(err, jc.ErrorIsNil)
	client.CheckCall(c, 0, "SetConfig", "core.https_address", "[::]")
}

func (s *utilsSuite) TestEnableHTTPSListenerError(c *gc.C) {
	var client mockConfigSetter
	client.SetErrors(errors.New("uh oh"))
	err := lxdclient.EnableHTTPSListener(&client)
	c.Assert(err, gc.ErrorMatches, "uh oh")
}

type mockConfigSetter struct {
	testing.Stub
}

func (m *mockConfigSetter) SetConfig(k, v string) error {
	m.MethodCall(m, "SetConfig", k, v)
	return m.NextErr()
}
