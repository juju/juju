// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

type MongoToolsSuite struct {
	env environs.Environ
	testing.LoggingSuite
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
	return environs.MongoStoragePath(version.CurrentSeries(), version.CurrentArch())
}

var mongoURLTests = []struct {
	summary        string   // a summary of the test purpose.
	contents       []string // names in private storage.
	publicContents []string // names in public storage.
	expect         string   // the name we expect to find (if no error).
	urlpart        string   // part of the url we expect to find (if not blank).
}{{
	summary:  "grab mongo from private storage if it exists there",
	contents: []string{currentMongoPath()},
	expect:   currentMongoPath(),
}, {
	summary: "fall back to public storage when nothing found in private",
	contents: []string{
		environs.MongoStoragePath("foo", version.CurrentArch()),
	},
	publicContents: []string{
		currentMongoPath(),
	},
	expect: "public-" + currentMongoPath(),
}, {
	summary: "if nothing in public or private storage, fall back to copy in ec2",
	contents: []string{
		environs.MongoStoragePath("foo", version.CurrentArch()),
		environs.MongoStoragePath(version.CurrentSeries(), "foo"),
	},
	publicContents: []string{
		environs.MongoStoragePath("foo", version.CurrentArch()),
	},
	urlpart: "http://juju-dist.s3.amazonaws.com",
},
}
