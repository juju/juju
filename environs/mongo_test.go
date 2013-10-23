// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/version"
)

type MongoToolsSuite struct {
	env environs.Environ
	testbase.LoggingSuite
	dataDir string
}

func (t *MongoToolsSuite) SetUpTest(c *gc.C) {
	t.LoggingSuite.SetUpTest(c)
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "test",
		"type":            "dummy",
		"state-server":    false,
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, gc.IsNil)
	t.env = env
	t.dataDir = c.MkDir()
}

func (t *MongoToolsSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	t.LoggingSuite.TearDownTest(c)
}

func currentMongoPath() string {
	return environs.MongoStoragePath(version.Current.Series, version.Current.Arch)
}
