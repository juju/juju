// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"fmt"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type ShowSuite struct {
	jujutesting.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore

	mockAPI *mockShowAPI
}

var _ = tc.Suite(&ShowSuite{})

func (s *ShowSuite) SetUpTest(c *tc.C) {
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
		applicationsInfoFunc: func([]names.ApplicationTag) ([]params.ApplicationInfoResult, error) { return nil, nil },
	}
}

func (s *ShowSuite) runShow(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, application.NewShowCommandForTest(s.mockAPI, s.store), args...)
}

type showTest struct {
	args   []string
	err    string
	stdout string
	stderr string
}

func (s *ShowSuite) assertRunShow(c *tc.C, t showTest) {
	context, err := s.runShow(c, t.args...)
	if t.err == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, tc.ErrorMatches, t.err)
	}
	c.Assert(cmdtesting.Stdout(context), tc.Equals, t.stdout)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, t.stderr)
}

func (s *ShowSuite) TestShowNoArguments(c *tc.C) {
	msg := "an application name must be supplied"
	s.assertRunShow(c, showTest{
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowSuite) TestShowInvalidName(c *tc.C) {
	msg := "application name so-42-far-not-good not valid"
	s.assertRunShow(c, showTest{
		args:   []string{"so-42-far-not-good"},
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowSuite) TestShowInvalidValidNames(c *tc.C) {
	msg := "application name so-42-far-not-good not valid"
	s.assertRunShow(c, showTest{
		args:   []string{"so-42-far-not-good", "wordpress"},
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowSuite) TestShowInvalidNames(c *tc.C) {
	msg := "application names so-42-far-not-good, oo/42 not valid"
	s.assertRunShow(c, showTest{
		args:   []string{"so-42-far-not-good", "oo/42"},
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowSuite) TestShowInvalidAndValidNames(c *tc.C) {
	msg := "application names so-42-far-not-good, oo/42 not valid"
	s.assertRunShow(c, showTest{
		args:   []string{"so-42-far-not-good", "wordpress", "oo/42"},
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowSuite) TestShowApiError(c *tc.C) {
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
		Base:        params.Base{Name: "ubuntu", Channel: "12.10"},
		Channel:     "development",
		Constraints: constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"),
		Principal:   true,
		Life:        state.Alive.String(),
		EndpointBindings: map[string]string{
			relation.JujuInfo: "myspace",
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

func (s *ShowSuite) TestShow(c *tc.C) {
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
  base: ubuntu@12.10
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
  life: alive
  endpoint-bindings:
    juju-info: myspace
`[1:],
	})
}

func (s *ShowSuite) TestShowJSON(c *tc.C) {
	s.mockAPI.applicationsInfoFunc = func([]names.ApplicationTag) ([]params.ApplicationInfoResult, error) {
		return []params.ApplicationInfoResult{
			{Result: s.createTestApplicationInfoWithExposedEndpoints("wordpress", "")},
		}, nil
	}
	s.assertRunShow(c, showTest{
		args:   []string{"wordpress", "--format", "json"},
		stdout: "{\"wordpress\":{\"charm\":\"charm-wordpress\",\"base\":\"ubuntu@12.10\",\"channel\":\"development\",\"constraints\":{\"arch\":\"amd64\",\"cores\":1,\"mem\":4096,\"root-disk\":8192},\"principal\":true,\"exposed\":false,\"exposed-endpoints\":{\"\":{\"expose-to-cidrs\":[\"192.168.0.0/24\"]},\"website\":{\"expose-to-spaces\":[\"non-euclidean-geometry\"]}},\"remote\":false,\"life\":\"alive\",\"endpoint-bindings\":{\"juju-info\":\"myspace\"}}}\n",
	})
}

func (s *ShowSuite) TestShowMix(c *tc.C) {
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

func (s *ShowSuite) TestShowMany(c *tc.C) {
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
  base: ubuntu@12.10
  channel: development
  constraints:
    arch: amd64
    cores: 1
    mem: 4096
    root-disk: 8192
  principal: true
  exposed: false
  remote: false
  life: alive
  endpoint-bindings:
    juju-info: myspace
wordpress:
  charm: charm-wordpress
  base: ubuntu@12.10
  channel: development
  constraints:
    arch: amd64
    cores: 1
    mem: 4096
    root-disk: 8192
  principal: true
  exposed: false
  remote: false
  life: alive
  endpoint-bindings:
    juju-info: myspace
`[1:],
	})
}

type mockShowAPI struct {
	applicationsInfoFunc func([]names.ApplicationTag) ([]params.ApplicationInfoResult, error)
}

func (s mockShowAPI) Close() error {
	return nil
}

func (s mockShowAPI) ApplicationsInfo(ctx context.Context, tags []names.ApplicationTag) ([]params.ApplicationInfoResult, error) {
	return s.applicationsInfoFunc(tags)
}
