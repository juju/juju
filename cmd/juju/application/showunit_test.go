// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"errors"
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiapplication "github.com/juju/juju/v3/api/client/application"
	"github.com/juju/juju/v3/cmd/juju/application"
	"github.com/juju/juju/v3/jujuclient"
	_ "github.com/juju/juju/v3/provider/dummy"
	"github.com/juju/juju/v3/state"
	jujutesting "github.com/juju/juju/v3/testing"
)

type ShowUnitSuite struct {
	jujutesting.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore

	mockAPI *mockShowUnitAPI
}

var _ = gc.Suite(&ShowUnitSuite{})

func (s *ShowUnitSuite) SetUpTest(c *gc.C) {
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

	s.mockAPI = &mockShowUnitAPI{
		unitsInfoFunc: func([]names.UnitTag) ([]apiapplication.UnitInfo, error) { return nil, nil },
	}
}

func (s *ShowUnitSuite) runShow(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, application.NewShowUnitCommandForTest(s.mockAPI, s.store), args...)
}

type showUnitTest struct {
	args   []string
	err    string
	stdout string
	stderr string
}

func (s *ShowUnitSuite) assertRunShow(c *gc.C, t showUnitTest) {
	context, err := s.runShow(c, t.args...)
	if t.err == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, t.err)
	}
	c.Assert(cmdtesting.Stdout(context), gc.Equals, t.stdout)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, t.stderr)
}

func (s *ShowUnitSuite) TestShowNoArguments(c *gc.C) {
	msg := "an unit name must be supplied"
	s.assertRunShow(c, showUnitTest{
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowUnitSuite) TestShowInvalidRelatedUnit(c *gc.C) {
	msg := "related unit name so-42-far-not-good not valid"
	s.assertRunShow(c, showUnitTest{
		args:   []string{"--related-unit", "so-42-far-not-good", "wordpress/0"},
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowUnitSuite) TestShowInvalidName(c *gc.C) {
	msg := "unit name so-42-far-not-good not valid"
	s.assertRunShow(c, showUnitTest{
		args:   []string{"so-42-far-not-good"},
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowUnitSuite) TestShowInvalidValidNames(c *gc.C) {
	msg := "unit name so-42-far-not-good not valid"
	s.assertRunShow(c, showUnitTest{
		args:   []string{"so-42-far-not-good", "wordpress/0"},
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowUnitSuite) TestShowInvalidNames(c *gc.C) {
	msg := "unit names so-42-far-not-good, oo not valid"
	s.assertRunShow(c, showUnitTest{
		args:   []string{"so-42-far-not-good", "oo"},
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowUnitSuite) TestShowInvalidAndValidNames(c *gc.C) {
	msg := "unit names so-42-far-not-good, oo not valid"
	s.assertRunShow(c, showUnitTest{
		args:   []string{"so-42-far-not-good", "wordpress/0", "oo"},
		err:    fmt.Sprintf("%v", msg),
		stderr: fmt.Sprintf("ERROR %v\n", msg),
	})
}

func (s *ShowUnitSuite) TestShowApiError(c *gc.C) {
	s.mockAPI.unitsInfoFunc = func([]names.UnitTag) ([]apiapplication.UnitInfo, error) {
		return []apiapplication.UnitInfo{{
			Error: errors.New("boom"),
		}}, nil
	}
	msg := "boom"
	s.assertRunShow(c, showUnitTest{
		args: []string{"wordpress/0"},
		err:  fmt.Sprintf("%v", msg),
	})
}

func (s *ShowUnitSuite) createTestUnitInfo(app string, otherEndpoint string) apiapplication.UnitInfo {
	result := apiapplication.UnitInfo{
		Tag:             fmt.Sprintf("unit-%v-0", app),
		WorkloadVersion: "666",
		Machine:         "0",
		OpenedPorts:     []string{"100-102/ip"},
		PublicAddress:   "10.0.0.1",
		Charm:           fmt.Sprintf("charm-%v", app),
		Leader:          true,
		Life:            state.Alive.String(),
		RelationData: []apiapplication.EndpointRelationData{{
			Endpoint:        "db",
			CrossModel:      true,
			RelatedEndpoint: "server",
			ApplicationData: map[string]interface{}{app: "setting"},
			UnitRelationData: map[string]apiapplication.RelationData{
				"mariadb/2": {
					InScope:  true,
					UnitData: map[string]interface{}{"mariadb/2": "mariadb/2-setting"},
				},
			},
		}},
		ProviderId: "provider-id",
		Address:    "192.168.1.1",
	}
	if otherEndpoint != "" {
		result.RelationData = append(result.RelationData, apiapplication.EndpointRelationData{
			Endpoint:        otherEndpoint,
			RelatedEndpoint: "common",
		})
		result.RelationData[0].UnitRelationData["mariadb/3"] = apiapplication.RelationData{
			InScope:  true,
			UnitData: map[string]interface{}{"mariadb/3": "mariadb/3-setting"},
		}
	}
	return result
}

func (s *ShowUnitSuite) TestShow(c *gc.C) {
	s.mockAPI.unitsInfoFunc = func([]names.UnitTag) ([]apiapplication.UnitInfo, error) {
		return []apiapplication.UnitInfo{
			s.createTestUnitInfo("wordpress", ""),
		}, nil
	}
	s.assertRunShow(c, showUnitTest{
		args: []string{"wordpress/0"},
		stdout: `
wordpress/0:
  workload-version: "666"
  machine: "0"
  opened-ports:
  - 100-102/ip
  public-address: 10.0.0.1
  charm: charm-wordpress
  leader: true
  life: alive
  relation-info:
  - relation-id: 0
    endpoint: db
    cross-model: true
    related-endpoint: server
    application-data:
      wordpress: setting
    related-units:
      mariadb/2:
        in-scope: true
        data:
          mariadb/2: mariadb/2-setting
  provider-id: provider-id
  address: 192.168.1.1
`[1:],
	})
}

func (s *ShowUnitSuite) TestShowAppOnly(c *gc.C) {
	s.mockAPI.unitsInfoFunc = func([]names.UnitTag) ([]apiapplication.UnitInfo, error) {
		return []apiapplication.UnitInfo{
			s.createTestUnitInfo("wordpress", ""),
		}, nil
	}
	s.assertRunShow(c, showUnitTest{
		args: []string{"wordpress/0", "--app"},
		stdout: `
wordpress/0:
  workload-version: "666"
  machine: "0"
  opened-ports:
  - 100-102/ip
  public-address: 10.0.0.1
  charm: charm-wordpress
  leader: true
  life: alive
  relation-info:
  - relation-id: 0
    endpoint: db
    cross-model: true
    related-endpoint: server
    application-data:
      wordpress: setting
  provider-id: provider-id
  address: 192.168.1.1
`[1:],
	})
}

func (s *ShowUnitSuite) TestShowEndpoint(c *gc.C) {
	s.mockAPI.unitsInfoFunc = func([]names.UnitTag) ([]apiapplication.UnitInfo, error) {
		return []apiapplication.UnitInfo{
			s.createTestUnitInfo("wordpress", "db-shared"),
		}, nil
	}
	s.assertRunShow(c, showUnitTest{
		args: []string{"wordpress/0", "--endpoint", "db-shared"},
		stdout: `
wordpress/0:
  workload-version: "666"
  machine: "0"
  opened-ports:
  - 100-102/ip
  public-address: 10.0.0.1
  charm: charm-wordpress
  leader: true
  life: alive
  relation-info:
  - relation-id: 0
    endpoint: db-shared
    related-endpoint: common
    application-data: {}
  provider-id: provider-id
  address: 192.168.1.1
`[1:],
	})
}

func (s *ShowUnitSuite) TestShowOtherUnit(c *gc.C) {
	s.mockAPI.unitsInfoFunc = func([]names.UnitTag) ([]apiapplication.UnitInfo, error) {
		return []apiapplication.UnitInfo{
			s.createTestUnitInfo("wordpress", "db-shared"),
		}, nil
	}
	s.assertRunShow(c, showUnitTest{
		args: []string{"wordpress/0", "--related-unit", "mariadb/3", "--endpoint", "db"},
		stdout: `
wordpress/0:
  workload-version: "666"
  machine: "0"
  opened-ports:
  - 100-102/ip
  public-address: 10.0.0.1
  charm: charm-wordpress
  leader: true
  life: alive
  relation-info:
  - relation-id: 0
    endpoint: db
    cross-model: true
    related-endpoint: server
    application-data:
      wordpress: setting
    related-units:
      mariadb/3:
        in-scope: true
        data:
          mariadb/3: mariadb/3-setting
  provider-id: provider-id
  address: 192.168.1.1
`[1:],
	})
}

func (s *ShowUnitSuite) TestShowJSON(c *gc.C) {
	s.mockAPI.unitsInfoFunc = func([]names.UnitTag) ([]apiapplication.UnitInfo, error) {
		return []apiapplication.UnitInfo{
			s.createTestUnitInfo("wordpress", ""),
		}, nil
	}
	s.assertRunShow(c, showUnitTest{
		args:   []string{"wordpress/0", "--format", "json"},
		stdout: `{"wordpress/0":{"workload-version":"666","machine":"0","opened-ports":["100-102/ip"],"public-address":"10.0.0.1","charm":"charm-wordpress","leader":true,"life":"alive","relation-info":[{"relation-id":0,"endpoint":"db","cross-model":true,"related-endpoint":"server","application-data":{"wordpress":"setting"},"local-unit":{"in-scope":false,"data":null},"related-units":{"mariadb/2":{"in-scope":true,"data":{"mariadb/2":"mariadb/2-setting"}}}}],"provider-id":"provider-id","address":"192.168.1.1"}}` + "\n",
	})
}

func (s *ShowUnitSuite) TestShowMix(c *gc.C) {
	s.mockAPI.unitsInfoFunc = func([]names.UnitTag) ([]apiapplication.UnitInfo, error) {
		return []apiapplication.UnitInfo{
			s.createTestUnitInfo("wordpress", ""),
			{Error: errors.New("boom")},
		}, nil
	}
	s.assertRunShow(c, showUnitTest{
		args: []string{"wordpress/0", "logging/0"},
		err:  "boom",
	})
}

func (s *ShowUnitSuite) TestShowMany(c *gc.C) {
	s.mockAPI.unitsInfoFunc = func([]names.UnitTag) ([]apiapplication.UnitInfo, error) {
		return []apiapplication.UnitInfo{
			s.createTestUnitInfo("wordpress", ""),
			s.createTestUnitInfo("logging", ""),
		}, nil
	}
	s.assertRunShow(c, showUnitTest{
		args: []string{"wordpress/0", "logging/0"},
		stdout: `
logging/0:
  workload-version: "666"
  machine: "0"
  opened-ports:
  - 100-102/ip
  public-address: 10.0.0.1
  charm: charm-logging
  leader: true
  life: alive
  relation-info:
  - relation-id: 0
    endpoint: db
    cross-model: true
    related-endpoint: server
    application-data:
      logging: setting
    related-units:
      mariadb/2:
        in-scope: true
        data:
          mariadb/2: mariadb/2-setting
  provider-id: provider-id
  address: 192.168.1.1
wordpress/0:
  workload-version: "666"
  machine: "0"
  opened-ports:
  - 100-102/ip
  public-address: 10.0.0.1
  charm: charm-wordpress
  leader: true
  life: alive
  relation-info:
  - relation-id: 0
    endpoint: db
    cross-model: true
    related-endpoint: server
    application-data:
      wordpress: setting
    related-units:
      mariadb/2:
        in-scope: true
        data:
          mariadb/2: mariadb/2-setting
  provider-id: provider-id
  address: 192.168.1.1
`[1:],
	})
}

type mockShowUnitAPI struct {
	unitsInfoFunc func([]names.UnitTag) ([]apiapplication.UnitInfo, error)
}

func (s mockShowUnitAPI) Close() error {
	return nil
}

func (s mockShowUnitAPI) UnitsInfo(tags []names.UnitTag) ([]apiapplication.UnitInfo, error) {
	return s.unitsInfoFunc(tags)
}
