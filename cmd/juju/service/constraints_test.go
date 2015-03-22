// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/service"
	"github.com/juju/juju/testing"
)

type ServiceConstraintsCommandsSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&ServiceConstraintsCommandsSuite{})

func (s *ServiceConstraintsCommandsSuite) TestSetInit(c *gc.C) {
	for _, test := range []struct {
		args []string
		err  string
	}{
		{
			args: []string{"--service", "mysql", "mem=4G"},
			err:  `flag provided but not defined: --service`,
		}, {
			args: []string{"-s", "mysql", "mem=4G"},
			err:  `flag provided but not defined: -s`,
		}, {
			args: []string{},
			err:  `no service name specified`,
		}, {
			args: []string{"mysql", "="},
			err:  `malformed constraint "="`,
		}, {
			args: []string{"cpu-power=250"},
			err:  `invalid service name "cpu-power=250"`,
		}, {
			args: []string{"mysql", "cpu-power=250"},
		},
	} {
		err := testing.InitCommand(&service.ServiceSetConstraintsCommand{}, test.args)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *ServiceConstraintsCommandsSuite) TestGetInit(c *gc.C) {
	for _, test := range []struct {
		args []string
		err  string
	}{
		{
			args: []string{"-s", "mysql"},
			err:  `flag provided but not defined: -s`,
		}, {
			args: []string{"--service", "mysql"},
			err:  `flag provided but not defined: --service`,
		}, {
			args: []string{},
			err:  `no service name specified`,
		}, {
			args: []string{"mysql-0"},
			err:  `invalid service name "mysql-0"`,
		}, {
			args: []string{"mysql"},
		},
	} {
		err := testing.InitCommand(&service.ServiceGetConstraintsCommand{}, test.args)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}
