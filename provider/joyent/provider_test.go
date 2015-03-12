// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/joyent"
	"github.com/juju/juju/testing"
)

type providerSimpleSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&providerSimpleSuite{})

func (*providerSimpleSuite) TestPrepareSetsControlDir(c *gc.C) {
	attrs := validAttrs()
	// drop the control-dir
	delete(attrs, "control-dir")
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	// Make sure the the value isn't set.
	_, ok := cfg.AllAttrs()["control-dir"]
	c.Assert(ok, jc.IsFalse)

	cfg, err = joyent.Provider.PrepareForCreateEnvironment(cfg)
	c.Assert(err, jc.ErrorIsNil)
	value, ok := cfg.AllAttrs()["control-dir"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Matches, "[a-f0-9]{32}")
}
