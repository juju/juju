// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/testing"
	"strings"
)

type OpenSuite struct{}

var _ = Suite(&OpenSuite{})

func (OpenSuite) TearDownTest(c *C) {
	dummy.Reset()
}

func (OpenSuite) TestNewDummyEnviron(c *C) {
	// matches *Settings.Map()
	config := map[string]interface{}{
		"name":            "foo",
		"type":            "dummy",
		"state-server":    false,
		"authorized-keys": "i-am-a-key",
		"admin-secret":    "foo",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	}
	env, err := environs.NewFromAttrs(config)
	c.Assert(err, IsNil)
	c.Assert(env.Bootstrap(constraints.Value{}), IsNil)
}

func (OpenSuite) TestNewUnknownEnviron(c *C) {
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "foo",
		"type":            "wondercloud",
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, ErrorMatches, "no registered provider for.*")
	c.Assert(env, IsNil)
}

func (OpenSuite) TestNewFromNameNoDefault(c *C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfigNoDefault, testing.SampleCertName).Restore()

	_, err := environs.NewFromName("")
	c.Assert(err, ErrorMatches, "no default environment found")
}

func (OpenSuite) TestNewFromNameGetDefault(c *C) {
	defer testing.MakeFakeHome(c, testing.SingleEnvConfig, testing.SampleCertName).Restore()

	e, err := environs.NewFromName("")
	c.Assert(err, IsNil)
	c.Assert(e.Name(), Equals, "erewhemos")
}

const checkEnv = `
environments:
    test:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`

type checkEnvironmentSuite struct{}

var _ = Suite(&checkEnvironmentSuite{})

func (s *checkEnvironmentSuite) TearDownTest(c *C) {
	dummy.Reset()
}

func (s *checkEnvironmentSuite) TestCheckEnvironment(c *C) {
	defer testing.MakeFakeHome(c, checkEnv, "existing").Restore()

	environ, err := environs.NewFromName("test")
	c.Assert(err, IsNil)

	// VerifyStorage is sufficient for our tests and much simpler
	// than Bootstrap which calls it.
	storage := environ.Storage()
	err = environs.VerifyStorage(storage)
	c.Assert(err, IsNil)
	err = environs.CheckEnvironment(environ)
	c.Assert(err, IsNil)
}

func (s *checkEnvironmentSuite) TestCheckEnvironmentFileNotFound(c *C) {
	defer testing.MakeFakeHome(c, checkEnv, "existing").Restore()

	environ, err := environs.NewFromName("test")
	c.Assert(err, IsNil)

	// VerifyStorage is sufficient for our tests and much simpler
	// than Bootstrap which calls it.
	storage := environ.Storage()
	err = environs.VerifyStorage(storage)
	c.Assert(err, IsNil)

	// When the bootstrap-verify file does not exist, it still believes
	// the environment is a juju-core one because earlier versions
	// did not create that file.
	err = storage.Remove("bootstrap-verify")
	c.Assert(err, IsNil)
	err = environs.CheckEnvironment(environ)
	c.Assert(err, IsNil)
}

func (s *checkEnvironmentSuite) TestCheckEnvironmentGetFails(c *C) {
	defer testing.MakeFakeHome(c, checkEnv, "existing").Restore()

	environ, err := environs.NewFromName("test")
	c.Assert(err, IsNil)

	// VerifyStorage is sufficient for our tests and much simpler
	// than Bootstrap which calls it.
	storage := environ.Storage()
	err = environs.VerifyStorage(storage)
	c.Assert(err, IsNil)

	// When fetching the verification file from storage fails,
	// we get an InvalidEnvironmentError.
	someError := errors.Unauthorizedf("you shall not pass")
	dummy.Poison(storage, "bootstrap-verify", someError)
	err = environs.CheckEnvironment(environ)
	c.Assert(err, Equals, someError)
}

func (s *checkEnvironmentSuite) TestCheckEnvironmentBadContent(c *C) {
	defer testing.MakeFakeHome(c, checkEnv, "existing").Restore()

	environ, err := environs.NewFromName("test")
	c.Assert(err, IsNil)

	// We mock a bad (eg. from a Python-juju environment) bootstrap-verify.
	storage := environ.Storage()
	content := "bad verification content"
	reader := strings.NewReader(content)
	err = storage.Put("bootstrap-verify", reader, int64(len(content)))
	c.Assert(err, IsNil)

	// When the bootstrap-verify file contains unexpected content,
	// we get an InvalidEnvironmentError.
	err = environs.CheckEnvironment(environ)
	c.Assert(err, Equals, environs.InvalidEnvironmentError)
}
