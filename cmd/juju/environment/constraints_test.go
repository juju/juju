// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/testing"
)

type EnvConstraintsCommandsSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&EnvConstraintsCommandsSuite{})

func (s *EnvConstraintsCommandsSuite) TestSetInit(c *gc.C) {
	for _, test := range []struct {
		args []string
		err  string
	}{
		{
			args: []string{"-s", "mysql"},
			err:  "flag provided but not defined: -s",
		}, {
			args: []string{"="},
			err:  `malformed constraint "="`,
		}, {
			args: []string{"cpu-power=250"},
		},
	} {
		err := testing.InitCommand(&environment.EnvSetConstraintsCommand{}, test.args)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *EnvConstraintsCommandsSuite) TestGetInit(c *gc.C) {
	for _, test := range []struct {
		args []string
		err  string
	}{
		{
			args: []string{"-s", "mysql"},
			err:  "flag provided but not defined: -s",
		}, {
			args: []string{"mysql"},
			err:  `unrecognized args: \["mysql"\]`,
		}, {
			args: []string{},
		},
	} {
		err := testing.InitCommand(&environment.EnvGetConstraintsCommand{}, test.args)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}
