// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/discoverspaces"
)

type ConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ConfigSuite{})

func (*ConfigSuite) TestAllSet(c *gc.C) {
	config := discoverspaces.Config{
		Facade:   fakeFacade{},
		Environ:  fakeEnviron{},
		NewName:  fakeNewName,
		Unlocker: fakeUnlocker{},
	}
	checkConfigValid(c, config)
}

func (*ConfigSuite) TestNilUnlocker(c *gc.C) {
	config := discoverspaces.Config{
		Facade:  fakeFacade{},
		Environ: fakeEnviron{},
		NewName: fakeNewName,
	}
	checkConfigValid(c, config)
}

func checkConfigValid(c *gc.C, config discoverspaces.Config) {
	c.Check(config.Validate(), jc.ErrorIsNil)
}

func (*ConfigSuite) TestNilFacade(c *gc.C) {
	config := discoverspaces.Config{
		Environ: fakeEnviron{},
		NewName: fakeNewName,
	}
	checkAlwaysInvalid(c, config, "nil Facade not valid")
}

func (*ConfigSuite) TestNilEnviron(c *gc.C) {
	config := discoverspaces.Config{
		Facade:  fakeFacade{},
		NewName: fakeNewName,
	}
	checkAlwaysInvalid(c, config, "nil Environ not valid")
}

func (*ConfigSuite) TestNilNewName(c *gc.C) {
	config := discoverspaces.Config{
		Facade:  fakeFacade{},
		Environ: fakeEnviron{},
	}
	checkAlwaysInvalid(c, config, "nil NewName not valid")
}

func checkAlwaysInvalid(c *gc.C, config discoverspaces.Config, message string) {
	check := func(err error) {
		c.Check(err.Error(), gc.Equals, message)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := discoverspaces.NewWorker(config)
	c.Check(worker, gc.IsNil)
	check(err)
}
