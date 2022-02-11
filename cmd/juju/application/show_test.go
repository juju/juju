// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	jujutesting "github.com/juju/juju/testing"
)

type ShowSuite struct {
	jujutesting.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore

	mockAPI *mockShowAPI
}

var _ = gc.Suite(&ShowSuite{})

func (s *ShowSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Models["testing"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/controller": {},
		},
		CurrentModel: "admin/controller",
	}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User: "admin",
	}

	s.mockAPI = &mockShowAPI{
		version:              9,
		applicationsInfoFunc: func([]names.ApplicationTag) ([]params.ApplicationInfoResult, error) { return nil, nil },
	}
}

func (s *ShowSuite) runShow(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, application.NewShowCommandForTest(s.mockAPI, s.store), args...)
}

type showTest struct {
	args   []string
	err    string
	stdout string
	stderr string
}

func (s *ShowSuite) assertRunShow(c *gc.C, t showTest) {
	context, err := s.runShow(c, t.args...)
	if t.err == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, t.err)
	}
	c.Assert(cmdtesting.Stdout(context), gc.Equals, t.stdout)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, t.stderr)
}

func (s *ShowSuite) TestShowNoArguments(c *gc.C) {
	msg := "an application name must be supplied"
	s.assertRunShow(c, showTest{
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowSuite) TestShowInvalidName(c *gc.C) {
	msg := "application name so-42-far-not-good not valid"
	s.assertRunShow(c, showTest{
		args:   []string{"so-42-far-not-good"},
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowSuite) TestShowInvalidValidNames(c *gc.C) {
	msg := "application name so-42-far-not-good not valid"
	s.assertRunShow(c, showTest{
		args:   []string{"so-42-far-not-good", "wordpress"},
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowSuite) TestShowInvalidNames(c *gc.C) {
	msg := "application names so-42-far-not-good, oo/42 not valid"
	s.assertRunShow(c, showTest{
		args:   []string{"so-42-far-not-good", "oo/42"},
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowSuite) TestShowInvalidAndValidNames(c *gc.C) {
	msg := "application names so-42-far-not-good, oo/42 not valid"
	s.assertRunShow(c, showTest{
		args:   []string{"so-42-far-not-good", "wordpress", "oo/42"},
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowSuite) TestShowUnsupported(c *gc.C) {
	s.mockAPI.version = 8
	s.assertRunShow(c, showTest{
		args: []string{"wordpress"},
		err:  "show applications on API server version 8 not supported",
	})
}

func (s *ShowSuite) TestShowApiError(c *gc.C) {
	s.mockAPI.applicationsInfoFunc = func([]names.ApplicationTag) ([]params.ApplicationInfoResult, error) {
		return []params.ApplicationInfoResult{
			{Error: &params.Error{Message: "boom"}},
		}, nil
	}
	msg := "boom"
	s.assertRunShow(c, showTest{
		args: []string{"wordpress"},
		err:  fmt.Sprintf("%v", msg),
	})
}

func (s *ShowSuite) createTestApplicationInfo(name string, suffix string) *params.ApplicationResult {
	app := fmt.Sprintf("%v%v", name, suffix)
	return &params.ApplicationResult{
		Tag:         fmt.Sprintf("application-%v", app),
		Charm:       fmt.Sprintf("charm-%v", app),
		Series:      "quantal",
		Channel:     "development",
		Constraints: constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"),
		Principal:   true,
		EndpointBindings: map[string]string{
			"juju-info": "myspace",
		},
	}
}

func (s *ShowSuite) createTestApplicationInfoWithExposedEndpoints(name string, suffix string) *params.ApplicationResult {
	app := s.createTestApplicationInfo(name, suffix)
	app.ExposedEndpoints = map[string]params.ExposedEndpoint{
		"": {
			ExposeToCIDRs: []string{"192.168.0.0/24"},
		},
		"website": {
			ExposeToSpaces: []string{"non-euclidean-geometry"},
		},
	}

	return app
}

func (s *ShowSuite) TestShow(c *gc.C) {
	s.mockAPI.applicationsInfoFunc = func([]names.ApplicationTag) ([]params.ApplicationInfoResult, error) {
		return []params.ApplicationInfoResult{
			{Result: s.createTestApplicationInfoWithExposedEndpoints("wordpress", "")},
		}, nil
	}
	s.assertRunShow(c, showTest{
		args: []string{"wordpress"},
		stdout: `
wordpress:
  charm: charm-wordpress
  series: quantal
  channel: development
  constraints:
    arch: amd64
    cores: 1
    mem: 4096
    root-disk: 8192
  principal: true
  exposed: false
  exposed-endpoints:
    "":
      expose-to-cidrs:
      - 192.168.0.0/24
    website:
      expose-to-spaces:
      - non-euclidean-geometry
  remote: false
  endpoint-bindings:
    juju-info: myspace
`[1:],
	})
}

func (s *ShowSuite) TestShowJSON(c *gc.C) {
	s.mockAPI.applicationsInfoFunc = func([]names.ApplicationTag) ([]params.ApplicationInfoResult, error) {
		return []params.ApplicationInfoResult{
			{Result: s.createTestApplicationInfoWithExposedEndpoints("wordpress", "")},
		}, nil
	}
	s.assertRunShow(c, showTest{
		args:   []string{"wordpress", "--format", "json"},
		stdout: "{\"wordpress\":{\"charm\":\"charm-wordpress\",\"series\":\"quantal\",\"channel\":\"development\",\"constraints\":{\"arch\":\"amd64\",\"cores\":1,\"mem\":4096,\"root-disk\":8192},\"principal\":true,\"exposed\":false,\"exposed-endpoints\":{\"\":{\"expose-to-cidrs\":[\"192.168.0.0/24\"]},\"website\":{\"expose-to-spaces\":[\"non-euclidean-geometry\"]}},\"remote\":false,\"endpoint-bindings\":{\"juju-info\":\"myspace\"}}}\n",
	})
}

func (s *ShowSuite) TestShowMix(c *gc.C) {
	s.mockAPI.applicationsInfoFunc = func([]names.ApplicationTag) ([]params.ApplicationInfoResult, error) {
		return []params.ApplicationInfoResult{
			{Result: s.createTestApplicationInfo("wordpress", "")},
			{Error: &params.Error{Message: "boom"}},
		}, nil
	}
	s.assertRunShow(c, showTest{
		args: []string{"wordpress", "logging"},
		err:  "boom",
	})
}

func (s *ShowSuite) TestShowMany(c *gc.C) {
	s.mockAPI.applicationsInfoFunc = func([]names.ApplicationTag) ([]params.ApplicationInfoResult, error) {
		return []params.ApplicationInfoResult{
			{Result: s.createTestApplicationInfo("wordpress", "")},
			{Result: s.createTestApplicationInfo("logging", "")},
		}, nil
	}
	s.assertRunShow(c, showTest{
		args: []string{"wordpress", "logging"},
		stdout: `
logging:
  charm: charm-logging
  series: quantal
  channel: development
  constraints:
    arch: amd64
    cores: 1
    mem: 4096
    root-disk: 8192
  principal: true
  exposed: false
  remote: false
  endpoint-bindings:
    juju-info: myspace
wordpress:
  charm: charm-wordpress
  series: quantal
  channel: development
  constraints:
    arch: amd64
    cores: 1
    mem: 4096
    root-disk: 8192
  principal: true
  exposed: false
  remote: false
  endpoint-bindings:
    juju-info: myspace
`[1:],
	})
}

type mockShowAPI struct {
	version              int
	applicationsInfoFunc func([]names.ApplicationTag) ([]params.ApplicationInfoResult, error)
}

func (s mockShowAPI) Close() error {
	return nil
}

func (s mockShowAPI) BestAPIVersion() int {
	return s.version
}

func (s mockShowAPI) ApplicationsInfo(tags []names.ApplicationTag) ([]params.ApplicationInfoResult, error) {
	return s.applicationsInfoFunc(tags)
}
