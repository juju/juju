// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/testing"
)

type ModelConstraintsCommandsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&ModelConstraintsCommandsSuite{})

func (s *ModelConstraintsCommandsSuite) TestSetInit(c *gc.C) {
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
		err := testing.InitCommand(model.NewModelSetConstraintsCommand(), test.args)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *ModelConstraintsCommandsSuite) TestGetInit(c *gc.C) {
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
		err := testing.InitCommand(model.NewModelGetConstraintsCommand(), test.args)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}
