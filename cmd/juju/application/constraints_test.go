// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type ApplicationConstraintsCommandsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&ApplicationConstraintsCommandsSuite{})

func (s *ApplicationConstraintsCommandsSuite) TestSetInit(c *gc.C) {
	for _, test := range []struct {
		args []string
		err  string
	}{{
		args: []string{"--application", "mysql", "mem=4G"},
		err:  `option provided but not defined: --application`,
	}, {
		args: []string{"-s", "mysql", "mem=4G"},
		err:  `option provided but not defined: -s`,
	}, {
		args: []string{},
		err:  `no application name specified`,
	}, {
		args: []string{"mysql", "="},
		err:  `malformed constraint "="`,
	}, {
		args: []string{"cpu-power=250"},
		err:  `invalid application name "cpu-power=250"`,
	}, {
		args: []string{"mysql", "cpu-power=250"},
	}} {
		cmd := application.NewApplicationSetConstraintsCommand()
		cmd.SetClientStore(jujuclienttesting.MinimalStore())
		err := cmdtesting.InitCommand(cmd, test.args)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *ApplicationConstraintsCommandsSuite) TestGetInit(c *gc.C) {
	for _, test := range []struct {
		args []string
		err  string
	}{
		{
			args: []string{"-s", "mysql"},
			err:  `option provided but not defined: -s`,
		}, {
			args: []string{"--application", "mysql"},
			err:  `option provided but not defined: --application`,
		}, {
			args: []string{},
			err:  `no application name specified`,
		}, {
			args: []string{"mysql-0"},
			err:  `invalid application name "mysql-0"`,
		}, {
			args: []string{"mysql"},
		},
	} {
		cmd := application.NewApplicationGetConstraintsCommand()
		cmd.SetClientStore(jujuclienttesting.MinimalStore())
		err := cmdtesting.InitCommand(cmd, test.args)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}
