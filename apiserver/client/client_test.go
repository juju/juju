// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"
	charmtesting "gopkg.in/juju/charm.v4/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
)

type clientSuite struct {
	baseSuite
}

type Killer interface {
	Kill() error
}

type serverSuite struct {
	baseSuite
	client *client.Client
}

var _ = gc.Suite(&serverSuite{})

func (s *serverSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	var err error
	auth := testing.FakeAuthorizer{
		Tag:            s.AdminUserTag(c),
		EnvironManager: true,
	}
	s.client, err = client.NewClient(s.State, common.NewResources(), auth)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) setAgentPresence(c *gc.C, machineId string) *presence.Pinger {
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	pinger, err := m.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	s.State.StartSync()
	err = m.WaitAgentPresence(coretesting.LongWait)
	c.Assert(err, jc.ErrorIsNil)
	return pinger
}

func (s *serverSuite) TestEnsureAvailabilityDeprecated(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	// We have to ensure the agents are alive, or EnsureAvailability will
	// create more to replace them.
	pingerA := s.setAgentPresence(c, "0")
	defer assertKill(c, pingerA)

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")

	arg := params.StateServersSpecs{[]params.StateServersSpec{{NumStateServers: 3}}}
	results, err := s.client.EnsureAvailability(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)
	ensureAvailabilityResult := result.Result
	c.Assert(ensureAvailabilityResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(ensureAvailabilityResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(ensureAvailabilityResult.Removed, gc.HasLen, 0)

	machines, err = s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")
	c.Assert(machines[1].Series(), gc.Equals, "quantal")
	c.Assert(machines[2].Series(), gc.Equals, "quantal")
}

func (s *serverSuite) TestShareEnvironmentAddMissingLocalFails(c *gc.C) {
	args := params.ModifyEnvironUsers{
		Changes: []params.ModifyEnvironUser{{
			UserTag: names.NewLocalUserTag("foobar").String(),
			Action:  params.AddEnvUser,
		}}}

	result, err := s.client.ShareEnvironment(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `could not share environment: user "foobar" does not exist locally: user "foobar" not found`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *serverSuite) TestUnshareEnvironment(c *gc.C) {
	user := s.Factory.MakeEnvUser(c, nil)
	_, err := s.State.EnvironmentUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)

	args := params.ModifyEnvironUsers{
		Changes: []params.ModifyEnvironUser{{
			UserTag: user.UserTag().String(),
			Action:  params.RemoveEnvUser,
		}}}

	result, err := s.client.ShareEnvironment(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	_, err = s.State.EnvironmentUser(user.UserTag())
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *serverSuite) TestShareEnvironmentAddLocalUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", NoEnvUser: true})
	args := params.ModifyEnvironUsers{
		Changes: []params.ModifyEnvironUser{{
			UserTag: user.Tag().String(),
			Action:  params.AddEnvUser,
		}}}

	result, err := s.client.ShareEnvironment(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	envUser, err := s.State.EnvironmentUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envUser.UserName(), gc.Equals, user.UserTag().Username())
	c.Assert(envUser.CreatedBy(), gc.Equals, dummy.AdminUserTag().Username())
	c.Assert(envUser.LastConnection(), gc.IsNil)
}

func (s *serverSuite) TestShareEnvironmentAddRemoteUser(c *gc.C) {
	user := names.NewUserTag("foobar@ubuntuone")
	args := params.ModifyEnvironUsers{
		Changes: []params.ModifyEnvironUser{{
			UserTag: user.String(),
			Action:  params.AddEnvUser,
		}}}

	result, err := s.client.ShareEnvironment(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	envUser, err := s.State.EnvironmentUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envUser.UserName(), gc.Equals, user.Username())
	c.Assert(envUser.CreatedBy(), gc.Equals, dummy.AdminUserTag().Username())
	c.Assert(envUser.LastConnection(), gc.IsNil)
}

func (s *serverSuite) TestShareEnvironmentInvalidTags(c *gc.C) {
	for _, testParam := range []struct {
		tag      string
		validTag bool
	}{{
		tag:      "unit-foo/0",
		validTag: true,
	}, {
		tag:      "service-foo",
		validTag: true,
	}, {
		tag:      "relation-wordpress:db mysql:db",
		validTag: true,
	}, {
		tag:      "machine-0",
		validTag: true,
	}, {
		tag:      "user@local",
		validTag: false,
	}, {
		tag:      "user-Mua^h^h^h^arh",
		validTag: true,
	}, {
		tag:      "user@",
		validTag: false,
	}, {
		tag:      "user@ubuntuone",
		validTag: false,
	}, {
		tag:      "user@ubuntuone",
		validTag: false,
	}, {
		tag:      "@ubuntuone",
		validTag: false,
	}, {
		tag:      "in^valid.",
		validTag: false,
	}, {
		tag:      "",
		validTag: false,
	},
	} {
		var expectedErr string
		errPart := `could not share environment: "` + regexp.QuoteMeta(testParam.tag) + `" is not a valid `

		if testParam.validTag {

			// The string is a valid tag, but not a user tag.
			expectedErr = errPart + `user tag`
		} else {

			// The string is not a valid tag of any kind.
			expectedErr = errPart + `tag`
		}

		args := params.ModifyEnvironUsers{
			Changes: []params.ModifyEnvironUser{{
				UserTag: testParam.tag,
				Action:  params.AddEnvUser,
			}}}

		_, err := s.client.ShareEnvironment(args)
		result, err := s.client.ShareEnvironment(args)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
		c.Assert(result.Results, gc.HasLen, 1)
		c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedErr)
	}
}

func (s *serverSuite) TestShareEnvironmentZeroArgs(c *gc.C) {
	args := params.ModifyEnvironUsers{Changes: []params.ModifyEnvironUser{{}}}

	_, err := s.client.ShareEnvironment(args)
	result, err := s.client.ShareEnvironment(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `could not share environment: "" is not a valid tag`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *serverSuite) TestShareEnvironmentInvalidAction(c *gc.C) {
	var dance params.EnvironAction = "dance"
	args := params.ModifyEnvironUsers{
		Changes: []params.ModifyEnvironUser{{
			UserTag: "user-user@local",
			Action:  dance,
		}}}

	_, err := s.client.ShareEnvironment(args)
	result, err := s.client.ShareEnvironment(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `unknown action "dance"`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *serverSuite) TestSetEnvironAgentVersion(c *gc.C) {
	args := params.SetEnvironAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetEnvironAgentVersion(args)
	c.Assert(err, jc.ErrorIsNil)

	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, found := envConfig.AllAttrs()["agent-version"]
	c.Assert(found, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, "9.8.7")
}

func (s *serverSuite) TestAbortCurrentUpgrade(c *gc.C) {
	// Create a provisioned state server.
	machine, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id("i-blah"), "fake-nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Start an upgrade.
	_, err = s.State.EnsureUpgradeInfo(
		machine.Id(),
		version.MustParse("1.2.3"),
		version.MustParse("9.8.7"),
	)
	c.Assert(err, jc.ErrorIsNil)
	isUpgrading, err := s.State.IsUpgrading()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isUpgrading, jc.IsTrue)

	// Abort it.
	err = s.client.AbortCurrentUpgrade()
	c.Assert(err, jc.ErrorIsNil)

	isUpgrading, err = s.State.IsUpgrading()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isUpgrading, jc.IsFalse)
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) TestClientStatus(c *gc.C) {
	s.setUpScenario(c)
	status, err := s.APIState.Client().Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, jc.DeepEquals, scenarioStatus)
}

func (s *clientSuite) TestCompatibleSettingsParsing(c *gc.C) {
	// Test the exported settings parsing in a compatible way.
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	service, err := s.State.Service("dummy")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, "local:quantal/dummy-1")

	// Empty string will be returned as nil.
	options := map[string]string{
		"title":    "foobar",
		"username": "",
	}
	settings, err := client.ParseSettingsCompatible(ch, options)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": nil,
	})

	// Illegal settings lead to an error.
	options = map[string]string{
		"yummy": "didgeridoo",
	}
	settings, err = client.ParseSettingsCompatible(ch, options)
	c.Assert(err, gc.ErrorMatches, `unknown option "yummy"`)
}

var (
	validSetTestValue     = "a value with spaces\nand newline\nand UTF-8 characters: \U0001F604 / \U0001F44D"
	invalidSetTestValue   = "a value with an invalid UTF-8 sequence: " + string([]byte{0xFF, 0xFF})
	correctedSetTestValue = "a value with an invalid UTF-8 sequence: \ufffd\ufffd"
)

func (s *clientSuite) TestClientServiceSet(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	err := s.APIState.Client().ServiceSet("dummy", map[string]string{
		"title":    "foobar",
		"username": validSetTestValue,
	})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": validSetTestValue,
	})

	// Test doesn't fail because Go JSON marshalling converts invalid
	// UTF-8 sequences transparently to U+FFFD. The test demonstrates
	// this behavior. It's a currently accepted behavior as it never has
	// been a real-life issue.
	err = s.APIState.Client().ServiceSet("dummy", map[string]string{
		"title":    "foobar",
		"username": invalidSetTestValue,
	})
	c.Assert(err, jc.ErrorIsNil)
	settings, err = dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": correctedSetTestValue,
	})

	err = s.APIState.Client().ServiceSet("dummy", map[string]string{
		"title":    "barfoo",
		"username": "",
	})
	c.Assert(err, jc.ErrorIsNil)
	settings, err = dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "barfoo",
		"username": "",
	})
}

func (s *clientSuite) TestClientServerUnset(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	err := s.APIState.Client().ServiceSet("dummy", map[string]string{
		"title":    "foobar",
		"username": "user name",
	})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": "user name",
	})

	err = s.APIState.Client().ServiceUnset("dummy", []string{"username"})
	c.Assert(err, jc.ErrorIsNil)
	settings, err = dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title": "foobar",
	})
}

func (s *clientSuite) TestClientServiceSetYAML(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	err := s.APIState.Client().ServiceSetYAML("dummy", "dummy:\n  title: foobar\n  username: user name\n")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": "user name",
	})

	err = s.APIState.Client().ServiceSetYAML("dummy", "dummy:\n  title: barfoo\n  username: \n")
	c.Assert(err, jc.ErrorIsNil)
	settings, err = dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title": "barfoo",
	})
}

var clientAddServiceUnitsTests = []struct {
	about    string
	service  string // if not set, defaults to 'dummy'
	expected []string
	to       string
	err      string
}{
	{
		about:    "returns unit names",
		expected: []string{"dummy/0", "dummy/1", "dummy/2"},
	},
	{
		about: "fails trying to add zero units",
		err:   "must add at least one unit",
	},
	{
		about:    "cannot mix to when adding multiple units",
		err:      "cannot use NumUnits with ToMachineSpec",
		expected: []string{"dummy/0", "dummy/1"},
		to:       "0",
	},
	{
		// Note: chained-state, we add 1 unit here, but the 3 units
		// from the first condition still exist
		about:    "force the unit onto bootstrap machine",
		expected: []string{"dummy/3"},
		to:       "0",
	},
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
}

func (s *clientSuite) TestClientAddServiceUnits(c *gc.C) {
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	for i, t := range clientAddServiceUnitsTests {
		c.Logf("test %d. %s", i, t.about)
		serviceName := t.service
		if serviceName == "" {
			serviceName = "dummy"
		}
		units, err := s.APIState.Client().AddServiceUnits(serviceName, len(t.expected), t.to)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(units, gc.DeepEquals, t.expected)
	}
	// Test that we actually assigned the unit to machine 0
	forcedUnit, err := s.BackingState.Unit("dummy/3")
	c.Assert(err, jc.ErrorIsNil)
	assignedMachine, err := forcedUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(assignedMachine, gc.Equals, "0")
}

func (s *clientSuite) TestClientAddUnitToMachineNotFound(c *gc.C) {
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	_, err := s.APIState.Client().AddServiceUnits("dummy", 1, "42")
	c.Assert(err, gc.ErrorMatches, `cannot add units for service "dummy" to machine 42: machine 42 not found`)
}

func (s *clientSuite) TestClientCharmInfo(c *gc.C) {
	var clientCharmInfoTests = []struct {
		about           string
		charm           string
		url             string
		expectedActions *charm.Actions
		err             string
	}{
		{
			about: "dummy charm which contains an expectedActions spec",
			charm: "dummy",
			url:   "local:quantal/dummy-1",
			expectedActions: &charm.Actions{
				ActionSpecs: map[string]charm.ActionSpec{
					"snapshot": charm.ActionSpec{
						Description: "Take a snapshot of the database.",
						Params: map[string]interface{}{
							"type":        "object",
							"description": "this boilerplate is insane, we have to fix it",
							"properties": map[string]interface{}{
								"outfile": map[string]interface{}{
									"default":     "foo.bz2",
									"description": "The file to write out to.",
									"type":        "string",
								},
							},
						},
					},
				},
			},
		},
		{
			about: "retrieves charm info",
			// Use wordpress for tests so that we can compare Provides and Requires.
			charm:           "wordpress",
			expectedActions: &charm.Actions{ActionSpecs: nil},
			url:             "local:quantal/wordpress-3",
		},
		{
			about:           "invalid URL",
			charm:           "wordpress",
			expectedActions: &charm.Actions{ActionSpecs: nil},
			url:             "not-valid",
			err:             "charm url series is not resolved",
		},
		{
			about:           "invalid schema",
			charm:           "wordpress",
			expectedActions: &charm.Actions{ActionSpecs: nil},
			url:             "not-valid:your-arguments",
			err:             `charm URL has invalid schema: "not-valid:your-arguments"`,
		},
		{
			about:           "unknown charm",
			charm:           "wordpress",
			expectedActions: &charm.Actions{ActionSpecs: nil},
			url:             "cs:missing/one-1",
			err:             `charm "cs:missing/one-1" not found`,
		},
	}

	for i, t := range clientCharmInfoTests {
		c.Logf("test %d. %s", i, t.about)
		charm := s.AddTestingCharm(c, t.charm)
		info, err := s.APIState.Client().CharmInfo(t.url)
		if t.err != "" {
			c.Check(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		expected := &api.CharmInfo{
			Revision: charm.Revision(),
			URL:      charm.URL().String(),
			Config:   charm.Config(),
			Meta:     charm.Meta(),
			Actions:  charm.Actions(),
		}
		c.Check(info, jc.DeepEquals, expected)
		c.Check(info.Actions, jc.DeepEquals, t.expectedActions)
	}
}

func (s *clientSuite) TestClientEnvironmentInfo(c *gc.C) {
	conf, _ := s.State.EnvironConfig()
	info, err := s.APIState.Client().EnvironmentInfo()
	c.Assert(err, jc.ErrorIsNil)
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.DefaultSeries, gc.Equals, config.PreferredSeries(conf))
	c.Assert(info.ProviderType, gc.Equals, conf.Type())
	c.Assert(info.Name, gc.Equals, conf.Name())
	c.Assert(info.UUID, gc.Equals, env.UUID())
}

var clientAnnotationsTests = []struct {
	about    string
	initial  map[string]string
	input    map[string]string
	expected map[string]string
	err      string
}{
	{
		about:    "test setting an annotation",
		input:    map[string]string{"mykey": "myvalue"},
		expected: map[string]string{"mykey": "myvalue"},
	},
	{
		about:    "test setting multiple annotations",
		input:    map[string]string{"key1": "value1", "key2": "value2"},
		expected: map[string]string{"key1": "value1", "key2": "value2"},
	},
	{
		about:    "test overriding annotations",
		initial:  map[string]string{"mykey": "myvalue"},
		input:    map[string]string{"mykey": "another-value"},
		expected: map[string]string{"mykey": "another-value"},
	},
	{
		about: "test setting an invalid annotation",
		input: map[string]string{"invalid.key": "myvalue"},
		err:   `cannot update annotations on .*: invalid key "invalid.key"`,
	},
}

func (s *clientSuite) TestClientAnnotations(c *gc.C) {
	// Set up entities.
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	environment, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	type taggedAnnotator interface {
		state.Annotator
		state.Entity
	}
	entities := []taggedAnnotator{service, unit, machine, environment}
	for i, t := range clientAnnotationsTests {
		for _, entity := range entities {
			id := entity.Tag().String() // this is WRONG, it should be Tag().Id() but the code is wrong.
			c.Logf("test %d. %s. entity %s", i, t.about, id)
			// Set initial entity annotations.
			err := entity.SetAnnotations(t.initial)
			c.Assert(err, jc.ErrorIsNil)
			// Add annotations using the API call.
			err = s.APIState.Client().SetAnnotations(id, t.input)
			if t.err != "" {
				c.Assert(err, gc.ErrorMatches, t.err)
				continue
			}
			// Check annotations are correctly set.
			dbann, err := entity.Annotations()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(dbann, gc.DeepEquals, t.expected)
			// Retrieve annotations using the API call.
			ann, err := s.APIState.Client().GetAnnotations(id)
			c.Assert(err, jc.ErrorIsNil)
			// Check annotations are correctly returned.
			c.Assert(ann, gc.DeepEquals, dbann)
			// Clean up annotations on the current entity.
			cleanup := make(map[string]string)
			for key := range dbann {
				cleanup[key] = ""
			}
			err = entity.SetAnnotations(cleanup)
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

func (s *clientSuite) TestClientAnnotationsBadEntity(c *gc.C) {
	bad := []string{"", "machine", "-foo", "foo-", "---", "machine-jim", "unit-123", "unit-foo", "service-", "service-foo/bar"}
	expected := `".*" is not a valid( [a-z]+)? tag`
	for _, id := range bad {
		err := s.APIState.Client().SetAnnotations(id, map[string]string{"mykey": "myvalue"})
		c.Assert(err, gc.ErrorMatches, expected)
		_, err = s.APIState.Client().GetAnnotations(id)
		c.Assert(err, gc.ErrorMatches, expected)
	}
}

var serviceExposeTests = []struct {
	about   string
	service string
	err     string
	exposed bool
}{
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
	{
		about:   "expose a service",
		service: "dummy-service",
		exposed: true,
	},
	{
		about:   "expose an already exposed service",
		service: "exposed-service",
		exposed: true,
	},
}

func (s *clientSuite) TestClientServiceExpose(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	serviceNames := []string{"dummy-service", "exposed-service"}
	svcs := make([]*state.Service, len(serviceNames))
	var err error
	for i, name := range serviceNames {
		svcs[i] = s.AddTestingService(c, name, charm)
		c.Assert(svcs[i].IsExposed(), jc.IsFalse)
	}
	err = svcs[1].SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svcs[1].IsExposed(), jc.IsTrue)
	for i, t := range serviceExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err = s.APIState.Client().ServiceExpose(t.service)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			service, err := s.State.Service(t.service)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(service.IsExposed(), gc.Equals, t.exposed)
		}
	}
}

var serviceUnexposeTests = []struct {
	about    string
	service  string
	err      string
	initial  bool
	expected bool
}{
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
	{
		about:    "unexpose a service",
		service:  "dummy-service",
		initial:  true,
		expected: false,
	},
	{
		about:    "unexpose an already unexposed service",
		service:  "dummy-service",
		initial:  false,
		expected: false,
	},
}

func (s *clientSuite) TestClientServiceUnexpose(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	for i, t := range serviceUnexposeTests {
		c.Logf("test %d. %s", i, t.about)
		svc := s.AddTestingService(c, "dummy-service", charm)
		if t.initial {
			svc.SetExposed()
		}
		c.Assert(svc.IsExposed(), gc.Equals, t.initial)
		err := s.APIState.Client().ServiceUnexpose(t.service)
		if t.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			svc.Refresh()
			c.Assert(svc.IsExposed(), gc.Equals, t.expected)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
		err = svc.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}
}

var serviceDestroyTests = []struct {
	about   string
	service string
	err     string
}{
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
	{
		about:   "destroy a service",
		service: "dummy-service",
	},
	{
		about:   "destroy an already destroyed service",
		service: "dummy-service",
		err:     `service "dummy-service" not found`,
	},
}

func (s *clientSuite) TestClientServiceDestroy(c *gc.C) {
	s.AddTestingService(c, "dummy-service", s.AddTestingCharm(c, "dummy"))
	for i, t := range serviceDestroyTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.APIState.Client().ServiceDestroy(t.service)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	// Now do ServiceDestroy on a service with units. Destroy will
	// cause the service to be not-Alive, but will not remove its
	// document.
	s.setUpScenario(c)
	serviceName := "wordpress"
	service, err := s.State.Service(serviceName)
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().ServiceDestroy(serviceName)
	c.Assert(err, jc.ErrorIsNil)
	err = service.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(service.Life(), gc.Not(gc.Equals), state.Alive)
}

func assertLife(c *gc.C, entity state.Living, life state.Life) {
	err := entity.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Life(), gc.Equals, life)
}

func assertRemoved(c *gc.C, entity state.Living) {
	err := entity.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func assertKill(c *gc.C, killer Killer) {
	c.Assert(killer.Kill(), gc.IsNil)
}

func (s *clientSuite) setupDestroyMachinesTest(c *gc.C) (*state.Machine, *state.Machine, *state.Machine, *state.Unit) {
	m0, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	m1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	m2, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	sch := s.AddTestingCharm(c, "wordpress")
	wordpress := s.AddTestingService(c, "wordpress", sch)
	u, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m1)
	c.Assert(err, jc.ErrorIsNil)

	return m0, m1, m2, u
}

func (s *clientSuite) TestDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)
	s.assertDestroyMachineSuccess(c, u, m0, m1, m2)
}

func (s *clientSuite) TestForceDestroyMachines(c *gc.C) {
	s.assertForceDestroyMachines(c)
}

func (s *clientSuite) TestDestroyPrincipalUnits(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	units := make([]*state.Unit, 5)
	for i := range units {
		unit, err := wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = unit.SetStatus(state.StatusStarted, "", nil)
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
	}
	s.assertDestroyPrincipalUnits(c, units)
}

func (s *clientSuite) TestDestroySubordinateUnits(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	// Try to destroy the subordinate alone; check it fails.
	err = s.APIState.Client().DestroyServiceUnits("logging/0")
	c.Assert(err, gc.ErrorMatches, `no units were destroyed: unit "logging/0" is a subordinate`)
	assertLife(c, logging0, state.Alive)

	s.assertDestroySubordinateUnits(c, wordpress0, logging0)
}

func (s *clientSuite) testClientUnitResolved(c *gc.C, retry bool, expectedResolvedMode state.ResolvedMode) {
	// Setup:
	s.setUpScenario(c)
	u, err := s.State.Unit("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetStatus(state.StatusError, "gaaah", nil)
	c.Assert(err, jc.ErrorIsNil)
	// Code under test:
	err = s.APIState.Client().Resolved("wordpress/0", retry)
	c.Assert(err, jc.ErrorIsNil)
	// Freshen the unit's state.
	err = u.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	// And now the actual test assertions: we set the unit as resolved via
	// the API so it should have a resolved mode set.
	mode := u.Resolved()
	c.Assert(mode, gc.Equals, expectedResolvedMode)
}

func (s *clientSuite) TestClientUnitResolved(c *gc.C) {
	s.testClientUnitResolved(c, false, state.ResolvedNoHooks)
}

func (s *clientSuite) TestClientUnitResolvedRetry(c *gc.C) {
	s.testClientUnitResolved(c, true, state.ResolvedRetryHooks)
}

func (s *clientSuite) TestClientServiceDeployCharmErrors(c *gc.C) {
	s.makeMockCharmStore()
	for url, expect := range map[string]string{
		"wordpress":                   "charm url series is not resolved",
		"cs:wordpress":                "charm url series is not resolved",
		"cs:precise/wordpress":        "charm url must include revision",
		"cs:precise/wordpress-999999": `cannot download charm ".*": charm not found in mock store: cs:precise/wordpress-999999`,
	} {
		c.Logf("test %s", url)
		err := s.APIState.Client().ServiceDeploy(
			url, "service", 1, "", constraints.Value{}, "",
		)
		c.Check(err, gc.ErrorMatches, expect)
		_, err = s.State.Service("service")
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
}

func (s *clientSuite) TestClientServiceDeployWithNetworks(c *gc.C) {
	s.makeMockCharmStore()
	curl, bundle := addCharm(c, "dummy")
	cons := constraints.MustParse("mem=4G networks=^net3")

	// Check for invalid network tags handling.
	err := s.APIState.Client().ServiceDeployWithNetworks(
		curl.String(), "service", 3, "", cons, "",
		[]string{"net1", "net2"},
	)
	c.Assert(err, gc.ErrorMatches, `"net1" is not a valid tag`)

	err = s.APIState.Client().ServiceDeployWithNetworks(
		curl.String(), "service", 3, "", cons, "",
		[]string{"network-net1", "network-net2"},
	)
	c.Assert(err, jc.ErrorIsNil)
	service := s.assertPrincipalDeployed(c, "service", curl, false, bundle, cons)

	networks, err := service.Networks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(networks, gc.DeepEquals, []string{"net1", "net2"})
	serviceCons, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serviceCons, gc.DeepEquals, cons)
}

func (s *clientSuite) assertPrincipalDeployed(c *gc.C, serviceName string, curl *charm.URL, forced bool, bundle charm.Charm, cons constraints.Value) *state.Service {
	service, err := s.State.Service(serviceName)
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, gc.Equals, forced)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	c.Assert(charm.Meta(), gc.DeepEquals, bundle.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, bundle.Config())

	serviceCons, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serviceCons, gc.DeepEquals, cons)
	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	for _, unit := range units {
		mid, err := unit.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		machine, err := s.State.Machine(mid)
		c.Assert(err, jc.ErrorIsNil)
		machineCons, err := machine.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(machineCons, gc.DeepEquals, cons)
	}
	return service
}

func (s *clientSuite) TestClientServiceDeployPrincipal(c *gc.C) {
	// TODO(fwereade): test ToMachineSpec directly on srvClient, when we
	// manage to extract it as a package and can thus do it conveniently.
	s.makeMockCharmStore()
	curl, bundle := addCharm(c, "dummy")
	mem4g := constraints.MustParse("mem=4G")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", mem4g, "",
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPrincipalDeployed(c, "service", curl, false, bundle, mem4g)
}

func (s *clientSuite) TestClientServiceDeploySubordinate(c *gc.C) {
	s.makeMockCharmStore()
	curl, bundle := addCharm(c, "logging")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 0, "", constraints.Value{}, "",
	)
	service, err := s.State.Service("service-name")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	c.Assert(charm.Meta(), gc.DeepEquals, bundle.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, bundle.Config())

	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)
}

func (s *clientSuite) TestClientServiceDeployConfig(c *gc.C) {
	// TODO(fwereade): test Config/ConfigYAML handling directly on srvClient.
	// Can't be done cleanly until it's extracted similarly to Machiner.
	s.makeMockCharmStore()
	curl, _ := addCharm(c, "dummy")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 1, "service-name:\n  username: fred", constraints.Value{}, "",
	)
	c.Assert(err, jc.ErrorIsNil)
	service, err := s.State.Service("service-name")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"username": "fred"})
}

func (s *clientSuite) TestClientServiceDeployConfigError(c *gc.C) {
	// TODO(fwereade): test Config/ConfigYAML handling directly on srvClient.
	// Can't be done cleanly until it's extracted similarly to Machiner.
	s.makeMockCharmStore()
	curl, _ := addCharm(c, "dummy")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 1, "service-name:\n  skill-level: fred", constraints.Value{}, "",
	)
	c.Assert(err, gc.ErrorMatches, `option "skill-level" expected int, got "fred"`)
	_, err = s.State.Service("service-name")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *clientSuite) TestClientServiceDeployToMachine(c *gc.C) {
	s.makeMockCharmStore()
	curl, bundle := addCharm(c, "dummy")

	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 1, "service-name:\n  username: fred", constraints.Value{}, machine.Id(),
	)
	c.Assert(err, jc.ErrorIsNil)

	service, err := s.State.Service("service-name")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	c.Assert(charm.Meta(), gc.DeepEquals, bundle.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, bundle.Config())

	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id())
}

func (s *clientSuite) TestClientServiceDeployToMachineNotFound(c *gc.C) {
	err := s.APIState.Client().ServiceDeploy(
		"cs:precise/service-name-1", "service-name", 1, "", constraints.Value{}, "42",
	)
	c.Assert(err, gc.ErrorMatches, `cannot deploy "service-name" to machine 42: machine 42 not found`)

	_, err = s.State.Service("service-name")
	c.Assert(err, gc.ErrorMatches, `service "service-name" not found`)
}

func (s *clientSuite) TestClientServiceDeployServiceOwner(c *gc.C) {
	s.makeMockCharmStore()
	curl, _ := addCharm(c, "dummy")

	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "password"})
	s.APIState = s.OpenAPIAs(c, user.Tag(), "password")

	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{}, "",
	)
	c.Assert(err, jc.ErrorIsNil)

	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(service.GetOwnerTag(), gc.Equals, user.Tag().String())
}

func (s *clientSuite) deployServiceForTests(c *gc.C) {
	curl, _ := addCharm(c, "dummy")
	err := s.APIState.Client().ServiceDeploy(curl.String(),
		"service", 1, "", constraints.Value{}, "",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) checkClientServiceUpdateSetCharm(c *gc.C, forceCharmUrl bool) {
	s.makeMockCharmStore()
	s.deployServiceForTests(c)
	addCharm(c, "wordpress")

	// Update the charm for the service.
	args := params.ServiceUpdate{
		ServiceName:   "service",
		CharmUrl:      "cs:precise/wordpress-3",
		ForceCharmUrl: forceCharmUrl,
	}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the charm has been updated and and the force flag correctly set.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, "cs:precise/wordpress-3")
	c.Assert(force, gc.Equals, forceCharmUrl)
}

func (s *clientSuite) TestClientServiceUpdateSetCharm(c *gc.C) {
	s.checkClientServiceUpdateSetCharm(c, false)
}

func (s *clientSuite) TestClientServiceUpdateForceSetCharm(c *gc.C) {
	s.checkClientServiceUpdateSetCharm(c, true)
}

func (s *clientSuite) TestClientServiceUpdateSetCharmErrors(c *gc.C) {
	s.makeMockCharmStore()
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	for charmUrl, expect := range map[string]string{
		"wordpress":                   "charm url series is not resolved",
		"cs:wordpress":                "charm url series is not resolved",
		"cs:precise/wordpress":        "charm url must include revision",
		"cs:precise/wordpress-999999": `cannot download charm ".*": charm not found in mock store: cs:precise/wordpress-999999`,
	} {
		c.Logf("test %s", charmUrl)
		args := params.ServiceUpdate{
			ServiceName: "wordpress",
			CharmUrl:    charmUrl,
		}
		err := s.APIState.Client().ServiceUpdate(args)
		c.Check(err, gc.ErrorMatches, expect)
	}
}

func (s *clientSuite) TestClientServiceUpdateSetMinUnits(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Set minimum units for the service.
	minUnits := 2
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		MinUnits:    &minUnits,
	}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the minimum number of units has been set.
	c.Assert(service.Refresh(), gc.IsNil)
	c.Assert(service.MinUnits(), gc.Equals, minUnits)
}

func (s *clientSuite) TestClientServiceUpdateSetMinUnitsError(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Set a negative minimum number of units for the service.
	minUnits := -1
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		MinUnits:    &minUnits,
	}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.ErrorMatches,
		`cannot set minimum units for service "dummy": cannot set a negative minimum number of units`)

	// Ensure the minimum number of units has not been set.
	c.Assert(service.Refresh(), gc.IsNil)
	c.Assert(service.MinUnits(), gc.Equals, 0)
}

func (s *clientSuite) TestClientServiceUpdateSetSettingsStrings(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update settings for the service.
	args := params.ServiceUpdate{
		ServiceName:     "dummy",
		SettingsStrings: map[string]string{"title": "s-title", "username": "s-user"},
	}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "s-title", "username": "s-user"}
	obtained, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, expected)
}

func (s *clientSuite) TestClientServiceUpdateSetSettingsYAML(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update settings for the service.
	args := params.ServiceUpdate{
		ServiceName:  "dummy",
		SettingsYAML: "dummy:\n  title: y-title\n  username: y-user",
	}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "y-title", "username": "y-user"}
	obtained, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, expected)
}

func (s *clientSuite) TestClientServiceUpdateSetConstraints(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update constraints for the service.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		Constraints: &cons,
	}
	err = s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientServiceUpdateAllParams(c *gc.C) {
	s.makeMockCharmStore()
	s.deployServiceForTests(c)
	addCharm(c, "wordpress")

	// Update all the service attributes.
	minUnits := 3
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	args := params.ServiceUpdate{
		ServiceName:     "service",
		CharmUrl:        "cs:precise/wordpress-3",
		ForceCharmUrl:   true,
		MinUnits:        &minUnits,
		SettingsStrings: map[string]string{"blog-title": "string-title"},
		SettingsYAML:    "service:\n  blog-title: yaml-title\n",
		Constraints:     &cons,
	}
	err = s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the service has been correctly updated.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)

	// Check the charm.
	ch, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, "cs:precise/wordpress-3")
	c.Assert(force, jc.IsTrue)

	// Check the minimum number of units.
	c.Assert(service.MinUnits(), gc.Equals, minUnits)

	// Check the settings: also ensure the YAML settings take precedence
	// over strings ones.
	expectedSettings := charm.Settings{"blog-title": "yaml-title"}
	obtainedSettings, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedSettings, gc.DeepEquals, expectedSettings)

	// Check the constraints.
	obtainedConstraints, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedConstraints, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientServiceUpdateNoParams(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	// Calling ServiceUpdate with no parameters set is a no-op.
	args := params.ServiceUpdate{ServiceName: "wordpress"}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestClientServiceUpdateNoService(c *gc.C) {
	err := s.APIState.Client().ServiceUpdate(params.ServiceUpdate{})
	c.Assert(err, gc.ErrorMatches, `"" is not a valid service name`)
}

func (s *clientSuite) TestClientServiceUpdateInvalidService(c *gc.C) {
	args := params.ServiceUpdate{ServiceName: "no-such-service"}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.ErrorMatches, `service "no-such-service" not found`)
}

func (s *clientSuite) TestClientServiceSetCharm(c *gc.C) {
	s.makeMockCharmStore()
	curl, _ := addCharm(c, "dummy")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{}, "",
	)
	c.Assert(err, jc.ErrorIsNil)
	addCharm(c, "wordpress")
	err = s.APIState.Client().ServiceSetCharm(
		"service", "cs:precise/wordpress-3", false,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that the charm is not marked as forced.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charm.URL().String(), gc.Equals, "cs:precise/wordpress-3")
	c.Assert(force, jc.IsFalse)
}

func (s *clientSuite) TestClientServiceSetCharmForce(c *gc.C) {
	s.makeMockCharmStore()
	curl, _ := addCharm(c, "dummy")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{}, "",
	)
	c.Assert(err, jc.ErrorIsNil)
	addCharm(c, "wordpress")
	err = s.APIState.Client().ServiceSetCharm(
		"service", "cs:precise/wordpress-3", true,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that the charm is marked as forced.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charm.URL().String(), gc.Equals, "cs:precise/wordpress-3")
	c.Assert(force, jc.IsTrue)
}

func (s *clientSuite) TestClientServiceSetCharmInvalidService(c *gc.C) {
	s.makeMockCharmStore()
	err := s.APIState.Client().ServiceSetCharm(
		"badservice", "cs:precise/wordpress-3", true,
	)
	c.Assert(err, gc.ErrorMatches, `service "badservice" not found`)
}

func (s *clientSuite) TestClientServiceSetCharmErrors(c *gc.C) {
	s.makeMockCharmStore()
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	for url, expect := range map[string]string{
		// TODO(fwereade,Makyo) make these errors consistent one day.
		"wordpress":                   "charm url series is not resolved",
		"cs:wordpress":                "charm url series is not resolved",
		"cs:precise/wordpress":        "charm url must include revision",
		"cs:precise/wordpress-999999": `cannot download charm ".*": charm not found in mock store: cs:precise/wordpress-999999`,
	} {
		c.Logf("test %s", url)
		err := s.APIState.Client().ServiceSetCharm(
			"wordpress", url, false,
		)
		c.Check(err, gc.ErrorMatches, expect)
	}
}

func (s *clientSuite) makeMockCharmStore() (store *charmtesting.MockCharmStore) {
	mockStore := charmtesting.NewMockCharmStore()
	origStore := client.CharmStore
	client.CharmStore = mockStore
	s.AddCleanup(func(_ *gc.C) { client.CharmStore = origStore })
	return mockStore
}

func addCharm(c *gc.C, name string) (*charm.URL, charm.Charm) {
	return addSeriesCharm(c, "precise", name)
}

func addSeriesCharm(c *gc.C, series, name string) (*charm.URL, charm.Charm) {
	bundle := testcharms.Repo.CharmArchive(c.MkDir(), name)
	scurl := fmt.Sprintf("cs:%s/%s-%d", series, name, bundle.Revision())
	curl := charm.MustParseURL(scurl)
	err := client.CharmStore.(*charmtesting.MockCharmStore).SetCharm(curl, bundle)
	c.Assert(err, jc.ErrorIsNil)
	return curl, bundle
}

func (s *clientSuite) checkEndpoints(c *gc.C, endpoints map[string]charm.Relation) {
	c.Assert(endpoints["wordpress"], gc.DeepEquals, charm.Relation{
		Name:      "db",
		Role:      charm.RelationRole("requirer"),
		Interface: "mysql",
		Optional:  false,
		Limit:     1,
		Scope:     charm.RelationScope("global"),
	})
	c.Assert(endpoints["mysql"], gc.DeepEquals, charm.Relation{
		Name:      "server",
		Role:      charm.RelationRole("provider"),
		Interface: "mysql",
		Optional:  false,
		Limit:     0,
		Scope:     charm.RelationScope("global"),
	})
}

func (s *clientSuite) assertAddRelation(c *gc.C, endpoints []string) {
	s.setUpScenario(c)
	res, err := s.APIState.Client().AddRelation(endpoints...)
	c.Assert(err, jc.ErrorIsNil)
	s.checkEndpoints(c, res.Endpoints)
	// Show that the relation was added.
	wpSvc, err := s.State.Service("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rels, err := wpSvc.Relations()
	// There are 2 relations - the logging-wordpress one set up in the
	// scenario and the one created in this test.
	c.Assert(len(rels), gc.Equals, 2)
	mySvc, err := s.State.Service("mysql")
	c.Assert(err, jc.ErrorIsNil)
	rels, err = mySvc.Relations()
	c.Assert(len(rels), gc.Equals, 1)
}

func (s *clientSuite) TestSuccessfullyAddRelation(c *gc.C) {
	endpoints := []string{"wordpress", "mysql"}
	s.assertAddRelation(c, endpoints)
}

func (s *clientSuite) TestSuccessfullyAddRelationSwapped(c *gc.C) {
	// Show that the order of the services listed in the AddRelation call
	// does not matter.  This is a repeat of the previous test with the service
	// names swapped.
	endpoints := []string{"mysql", "wordpress"}
	s.assertAddRelation(c, endpoints)
}

func (s *clientSuite) TestCallWithOnlyOneEndpoint(c *gc.C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress"}
	_, err := s.APIState.Client().AddRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *clientSuite) TestCallWithOneEndpointTooMany(c *gc.C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress", "mysql", "logging"}
	_, err := s.APIState.Client().AddRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, "cannot relate 3 endpoints")
}

func (s *clientSuite) TestAddAlreadyAddedRelation(c *gc.C) {
	s.setUpScenario(c)
	// Add a relation between wordpress and mysql.
	endpoints := []string{"wordpress", "mysql"}
	eps, err := s.State.InferEndpoints(endpoints...)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	// And try to add it again.
	_, err = s.APIState.Client().AddRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation already exists`)
}

func (s *clientSuite) setupRelationScenario(c *gc.C, endpoints []string) *state.Relation {
	s.setUpScenario(c)
	// Add a relation between the endpoints.
	eps, err := s.State.InferEndpoints(endpoints...)
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	return relation
}

func (s *clientSuite) assertDestroyRelation(c *gc.C, endpoints []string) {
	s.assertDestroyRelationSuccess(
		c,
		s.setupRelationScenario(c, endpoints),
		endpoints)
}

func (s *clientSuite) assertDestroyRelationSuccess(c *gc.C, relation *state.Relation, endpoints []string) {
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, jc.ErrorIsNil)
	// Show that the relation was removed.
	c.Assert(relation.Refresh(), jc.Satisfies, errors.IsNotFound)
}

func (s *clientSuite) TestSuccessfulDestroyRelation(c *gc.C) {
	endpoints := []string{"wordpress", "mysql"}
	s.assertDestroyRelation(c, endpoints)
}

func (s *clientSuite) TestSuccessfullyDestroyRelationSwapped(c *gc.C) {
	// Show that the order of the services listed in the DestroyRelation call
	// does not matter.  This is a repeat of the previous test with the service
	// names swapped.
	endpoints := []string{"mysql", "wordpress"}
	s.assertDestroyRelation(c, endpoints)
}

func (s *clientSuite) TestNoRelation(c *gc.C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress", "mysql"}
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, `relation "wordpress:db mysql:server" not found`)
}

func (s *clientSuite) TestAttemptDestroyingNonExistentRelation(c *gc.C) {
	s.setUpScenario(c)
	s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))
	endpoints := []string{"riak", "wordpress"}
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *clientSuite) TestAttemptDestroyingWithOnlyOneEndpoint(c *gc.C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress"}
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *clientSuite) TestAttemptDestroyingPeerRelation(c *gc.C) {
	s.setUpScenario(c)
	s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))

	endpoints := []string{"riak:ring"}
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, `cannot destroy relation "riak:ring": is a peer relation`)
}

func (s *clientSuite) TestAttemptDestroyingAlreadyDestroyedRelation(c *gc.C) {
	s.setUpScenario(c)

	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := []string{"wordpress", "mysql"}
	err = s.APIState.Client().DestroyRelation(endpoints...)
	// Show that the relation was removed.
	c.Assert(rel.Refresh(), jc.Satisfies, errors.IsNotFound)

	// And try to destroy it again.
	err = s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, `relation "wordpress:db mysql:server" not found`)
}

func (s *clientSuite) TestClientWatchAll(c *gc.C) {
	// A very simple end-to-end test, because
	// all the logic is tested elsewhere.
	m, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetProvisioned("i-0", agent.BootstrapNonce, nil)
	c.Assert(err, jc.ErrorIsNil)
	watcher, err := s.APIState.Client().WatchAll()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := watcher.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}()
	deltas, err := watcher.Next()
	c.Assert(err, jc.ErrorIsNil)
	if !c.Check(deltas, gc.DeepEquals, []multiwatcher.Delta{{
		Entity: &multiwatcher.MachineInfo{
			Id:                      m.Id(),
			InstanceId:              "i-0",
			Status:                  multiwatcher.StatusPending,
			Life:                    multiwatcher.Alive,
			Series:                  "quantal",
			Jobs:                    []multiwatcher.MachineJob{state.JobManageEnviron.ToParams()},
			Addresses:               []network.Address{},
			HardwareCharacteristics: &instance.HardwareCharacteristics{},
		},
	}}) {
		c.Logf("got:")
		for _, d := range deltas {
			c.Logf("%#v\n", d.Entity)
		}
	}
}

func (s *clientSuite) TestClientSetServiceConstraints(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update constraints for the service.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().SetServiceConstraints("dummy", cons)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientGetServiceConstraints(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Set constraints for the service.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = service.SetConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)

	// Check we can get the constraints.
	obtained, err := s.APIState.Client().GetServiceConstraints("dummy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientSetEnvironmentConstraints(c *gc.C) {
	// Set constraints for the environment.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().SetEnvironmentConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := s.State.EnvironConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientGetEnvironmentConstraints(c *gc.C) {
	// Set constraints for the environment.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetEnvironConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)

	// Check we can get the constraints.
	obtained, err := s.APIState.Client().GetEnvironmentConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientServiceCharmRelations(c *gc.C) {
	s.setUpScenario(c)
	_, err := s.APIState.Client().ServiceCharmRelations("blah")
	c.Assert(err, gc.ErrorMatches, `service "blah" not found`)

	relations, err := s.APIState.Client().ServiceCharmRelations("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relations, gc.DeepEquals, []string{
		"cache", "db", "juju-info", "logging-dir", "monitoring-port", "url",
	})
}

func (s *clientSuite) TestClientPublicAddressErrors(c *gc.C) {
	s.setUpScenario(c)
	_, err := s.APIState.Client().PublicAddress("wordpress")
	c.Assert(err, gc.ErrorMatches, `unknown unit or machine "wordpress"`)
	_, err = s.APIState.Client().PublicAddress("0")
	c.Assert(err, gc.ErrorMatches, `machine "0" has no public address`)
	_, err = s.APIState.Client().PublicAddress("wordpress/0")
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" has no public address`)
}

func (s *clientSuite) TestClientPublicAddressMachine(c *gc.C) {
	s.setUpScenario(c)

	// Internally, network.SelectPublicAddress is used; the "most public"
	// address is returned.
	m1, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	cloudLocalAddress := network.NewAddress("cloudlocal", network.ScopeCloudLocal)
	publicAddress := network.NewAddress("public", network.ScopePublic)
	err = m1.SetAddresses(cloudLocalAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PublicAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "cloudlocal")
	err = m1.SetAddresses(cloudLocalAddress, publicAddress)
	addr, err = s.APIState.Client().PublicAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "public")
}

func (s *clientSuite) TestClientPublicAddressUnit(c *gc.C) {
	s.setUpScenario(c)

	m1, err := s.State.Machine("1")
	publicAddress := network.NewAddress("public", network.ScopePublic)
	err = m1.SetAddresses(publicAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PublicAddress("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "public")
}

func (s *clientSuite) TestClientPrivateAddressErrors(c *gc.C) {
	s.setUpScenario(c)
	_, err := s.APIState.Client().PrivateAddress("wordpress")
	c.Assert(err, gc.ErrorMatches, `unknown unit or machine "wordpress"`)
	_, err = s.APIState.Client().PrivateAddress("0")
	c.Assert(err, gc.ErrorMatches, `machine "0" has no internal address`)
	_, err = s.APIState.Client().PrivateAddress("wordpress/0")
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" has no internal address`)
}

func (s *clientSuite) TestClientPrivateAddress(c *gc.C) {
	s.setUpScenario(c)

	// Internally, network.SelectInternalAddress is used; the public
	// address if no cloud-local one is available.
	m1, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	cloudLocalAddress := network.NewAddress("cloudlocal", network.ScopeCloudLocal)
	publicAddress := network.NewAddress("public", network.ScopePublic)
	err = m1.SetAddresses(publicAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PrivateAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "public")
	err = m1.SetAddresses(cloudLocalAddress, publicAddress)
	addr, err = s.APIState.Client().PrivateAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "cloudlocal")
}

func (s *clientSuite) TestClientPrivateAddressUnit(c *gc.C) {
	s.setUpScenario(c)

	m1, err := s.State.Machine("1")
	privateAddress := network.NewAddress("private", network.ScopeCloudLocal)
	err = m1.SetAddresses(privateAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PrivateAddress("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "private")
}

func (s *clientSuite) TestClientEnvironmentGet(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	attrs, err := s.APIState.Client().EnvironmentGet()
	c.Assert(err, jc.ErrorIsNil)
	allAttrs := envConfig.AllAttrs()
	// We cannot simply use DeepEquals, because after the
	// map[string]interface{} result of EnvironmentGet is
	// serialized to JSON, integers are converted to floats.
	for key, apiValue := range attrs {
		envValue, found := allAttrs[key]
		c.Check(found, jc.IsTrue)
		switch apiValue.(type) {
		case float64, float32:
			c.Check(fmt.Sprintf("%v", envValue), gc.Equals, fmt.Sprintf("%v", apiValue))
		default:
			c.Check(envValue, gc.Equals, apiValue)
		}
	}
}

func (s *clientSuite) TestClientEnvironmentSet(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found := envConfig.AllAttrs()["some-key"]
	c.Assert(found, jc.IsFalse)

	args := map[string]interface{}{"some-key": "value"}
	err = s.APIState.Client().EnvironmentSet(args)
	c.Assert(err, jc.ErrorIsNil)

	envConfig, err = s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	value, found := envConfig.AllAttrs()["some-key"]
	c.Assert(found, jc.IsTrue)
	c.Assert(value, gc.Equals, "value")
}

func (s *clientSuite) TestClientEnvironmentSetDeprecated(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	url := envConfig.AllAttrs()["agent-metadata-url"]
	c.Assert(url, gc.Equals, "")

	args := map[string]interface{}{"tools-metadata-url": "value"}
	err = s.APIState.Client().EnvironmentSet(args)
	c.Assert(err, jc.ErrorIsNil)

	envConfig, err = s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	value, found := envConfig.AllAttrs()["agent-metadata-url"]
	c.Assert(found, jc.IsTrue)
	c.Assert(value, gc.Equals, "value")
	value, found = envConfig.AllAttrs()["tools-metadata-url"]
	c.Assert(found, jc.IsTrue)
	c.Assert(value, gc.Equals, "value")
}

func (s *clientSuite) TestClientEnvironmentSetCannotChangeAgentVersion(c *gc.C) {
	args := map[string]interface{}{"agent-version": "9.9.9"}
	err := s.APIState.Client().EnvironmentSet(args)
	c.Assert(err, gc.ErrorMatches, "agent-version cannot be changed")
	// It's okay to pass env back with the same agent-version.
	cfg, err := s.APIState.Client().EnvironmentGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg["agent-version"], gc.NotNil)
	err = s.APIState.Client().EnvironmentSet(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestClientEnvironmentUnset(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"abc": 123}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found := envConfig.AllAttrs()["abc"]
	c.Assert(found, jc.IsTrue)

	err = s.APIState.Client().EnvironmentUnset("abc")
	c.Assert(err, jc.ErrorIsNil)
	envConfig, err = s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found = envConfig.AllAttrs()["abc"]
	c.Assert(found, jc.IsFalse)
}

func (s *clientSuite) TestClientEnvironmentUnsetMissing(c *gc.C) {
	// It's okay to unset a non-existent attribute.
	err := s.APIState.Client().EnvironmentUnset("not_there")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestClientEnvironmentUnsetError(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"abc": 123}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found := envConfig.AllAttrs()["abc"]
	c.Assert(found, jc.IsTrue)

	// "type" may not be removed, and this will cause an error.
	// If any one attribute's removal causes an error, there
	// should be no change.
	err = s.APIState.Client().EnvironmentUnset("abc", "type")
	c.Assert(err, gc.ErrorMatches, "type: expected string, got nothing")
	envConfig, err = s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found = envConfig.AllAttrs()["abc"]
	c.Assert(found, jc.IsTrue)
}

func (s *clientSuite) TestClientFindTools(c *gc.C) {
	result, err := s.APIState.Client().FindTools(2, -1, "", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, jc.Satisfies, params.IsCodeNotFound)
	toolstesting.UploadToStorage(c, s.DefaultToolsStorage, "released", version.MustParseBinary("2.12.0-precise-amd64"))
	result, err = s.APIState.Client().FindTools(2, 12, "precise", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.List, gc.HasLen, 1)
	c.Assert(result.List[0].Version, gc.Equals, version.MustParseBinary("2.12.0-precise-amd64"))
	url := fmt.Sprintf("https://%s/environment/90168e4c-2f10-4e9c-83c2-feedfacee5a9/tools/%s", s.APIState.Addr(), result.List[0].Version)
	c.Assert(result.List[0].URL, gc.Equals, url)
}

func (s *clientSuite) checkMachine(c *gc.C, id, series, cons string) {
	// Ensure the machine was actually created.
	machine, err := s.BackingState.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Series(), gc.Equals, series)
	c.Assert(machine.Jobs(), gc.DeepEquals, []state.MachineJob{state.JobHostUnits})
	machineConstraints, err := machine.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineConstraints.String(), gc.Equals, cons)
}

func (s *clientSuite) TestClientAddMachinesDefaultSeries(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		}
	}
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
		s.checkMachine(c, machineResult.Machine, coretesting.FakeDefaultSeries, apiParams[i].Constraints.String())
	}
}

func (s *clientSuite) TestClientAddMachinesWithSeries(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Series: "quantal",
			Jobs:   []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		}
	}
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
		s.checkMachine(c, machineResult.Machine, "quantal", apiParams[i].Constraints.String())
	}
}

func (s *clientSuite) TestClientAddMachineInsideMachine(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{{
		Jobs:          []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		ContainerType: instance.LXC,
		ParentId:      "0",
		Series:        "quantal",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Machine, gc.Equals, "0/lxc/0")
}

func (s *clientSuite) TestClientAddMachinesWithConstraints(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		}
	}
	// The last machine has some constraints.
	apiParams[2].Constraints = constraints.MustParse("mem=4G")
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
		s.checkMachine(c, machineResult.Machine, coretesting.FakeDefaultSeries, apiParams[i].Constraints.String())
	}
}

func (s *clientSuite) TestClientAddMachinesWithPlacement(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 4)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		}
	}
	apiParams[0].Placement = instance.MustParsePlacement("lxc")
	apiParams[1].Placement = instance.MustParsePlacement("lxc:0")
	apiParams[1].ContainerType = instance.LXC
	apiParams[2].Placement = instance.MustParsePlacement("dummyenv:invalid")
	apiParams[3].Placement = instance.MustParsePlacement("dummyenv:valid")
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 4)
	c.Assert(machines[0].Machine, gc.Equals, "0/lxc/0")
	c.Assert(machines[1].Error, gc.ErrorMatches, "container type and placement are mutually exclusive")
	c.Assert(machines[2].Error, gc.ErrorMatches, "cannot add a new machine: invalid placement is invalid")
	c.Assert(machines[3].Machine, gc.Equals, "1")

	m, err := s.BackingState.Machine(machines[3].Machine)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Placement(), gc.DeepEquals, apiParams[3].Placement.Directive)
}

func (s *clientSuite) TestClientAddMachines1dot18(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 2)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		}
	}
	apiParams[1].ContainerType = instance.LXC
	apiParams[1].ParentId = "0"
	machines, err := s.APIState.Client().AddMachines1dot18(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 2)
	c.Assert(machines[0].Machine, gc.Equals, "0")
	c.Assert(machines[1].Machine, gc.Equals, "0/lxc/0")
}

func (s *clientSuite) TestClientAddMachines1dot18SomeErrors(c *gc.C) {
	apiParams := []params.AddMachineParams{{
		Jobs:     []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		ParentId: "123",
	}}
	machines, err := s.APIState.Client().AddMachines1dot18(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 1)
	c.Check(machines[0].Error, gc.ErrorMatches, "parent machine specified without container type")
}

func (s *clientSuite) TestClientAddMachinesSomeErrors(c *gc.C) {
	// Here we check that adding a number of containers correctly handles the
	// case that some adds succeed and others fail and report the errors
	// accordingly.
	// We will set up params to the AddMachines API to attempt to create 3 machines.
	// Machines 0 and 1 will be added successfully.
	// Remaining machines will fail due to different reasons.

	// Create a machine to host the requested containers.
	host, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	// The host only supports lxc containers.
	err = host.SetSupportedContainers([]instance.ContainerType{instance.LXC})
	c.Assert(err, jc.ErrorIsNil)

	// Set up params for adding 3 containers.
	apiParams := make([]params.AddMachineParams, 3)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		}
	}
	// This will cause a machine add to fail due to an unsupported container.
	apiParams[2].ContainerType = instance.KVM
	apiParams[2].ParentId = host.Id()
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)

	// Check the results - machines 2 and 3 will have errors.
	c.Check(machines[0].Machine, gc.Equals, "1")
	c.Check(machines[0].Error, gc.IsNil)
	c.Check(machines[1].Machine, gc.Equals, "2")
	c.Check(machines[1].Error, gc.IsNil)
	c.Check(machines[2].Error, gc.ErrorMatches, "cannot add a new machine: machine 0 cannot host kvm containers")
}

func (s *clientSuite) TestClientAddMachinesWithInstanceIdSomeErrors(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	addrs := []network.Address{network.NewAddress("1.2.3.4", network.ScopeUnknown)}
	hc := instance.MustParseHardware("mem=4G")
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
			InstanceId: instance.Id(fmt.Sprintf("1234-%d", i)),
			Nonce:      "foo",
			HardwareCharacteristics: hc,
			Addrs: addrs,
		}
	}
	// This will cause the last machine add to fail.
	apiParams[2].Nonce = ""
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		if i == 2 {
			c.Assert(machineResult.Error, gc.NotNil)
			c.Assert(machineResult.Error, gc.ErrorMatches, "cannot add a new machine: cannot add a machine with an instance id and no nonce")
		} else {
			c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
			s.checkMachine(c, machineResult.Machine, coretesting.FakeDefaultSeries, apiParams[i].Constraints.String())
			instanceId := fmt.Sprintf("1234-%d", i)
			s.checkInstance(c, machineResult.Machine, instanceId, "foo", hc, addrs)
		}
	}
}

func (s *clientSuite) checkInstance(c *gc.C, id, instanceId, nonce string,
	hc instance.HardwareCharacteristics, addr []network.Address) {

	machine, err := s.BackingState.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	machineInstanceId, err := machine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.CheckProvisioned(nonce), jc.IsTrue)
	c.Assert(machineInstanceId, gc.Equals, instance.Id(instanceId))
	machineHardware, err := machine.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineHardware.String(), gc.Equals, hc.String())
	c.Assert(machine.Addresses(), gc.DeepEquals, addr)
}

func (s *clientSuite) TestInjectMachinesStillExists(c *gc.C) {
	results := new(params.AddMachinesResults)
	// We need to use Call directly because the client interface
	// no longer refers to InjectMachine.
	args := params.AddMachines{
		MachineParams: []params.AddMachineParams{{
			Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
			InstanceId: "i-foo",
			Nonce:      "nonce",
		}},
	}
	err := s.APIState.APICall("Client", 0, "", "AddMachines", args, &results)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Machines, gc.HasLen, 1)
}

func (s *clientSuite) TestProvisioningScript(c *gc.C) {
	// Inject a machine and then call the ProvisioningScript API.
	// The result should be the same as when calling MachineConfig,
	// converting it to a cloudinit.MachineConfig, and disabling
	// apt_upgrade.
	apiParams := params.AddMachineParams{
		Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
		HardwareCharacteristics: instance.MustParseHardware("arch=amd64"),
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 1)
	machineId := machines[0].Machine
	// Call ProvisioningScript. Normally ProvisioningScript and
	// MachineConfig are mutually exclusive; both of them will
	// allocate a api password for the machine agent.
	script, err := s.APIState.Client().ProvisioningScript(params.ProvisioningScriptParams{
		MachineId: machineId,
		Nonce:     apiParams.Nonce,
	})
	c.Assert(err, jc.ErrorIsNil)
	mcfg, err := client.MachineConfig(s.State, machineId, apiParams.Nonce, "")
	c.Assert(err, jc.ErrorIsNil)
	sshinitScript, err := manual.ProvisioningScript(mcfg)
	c.Assert(err, jc.ErrorIsNil)
	// ProvisioningScript internally calls MachineConfig,
	// which allocates a new, random password. Everything
	// about the scripts should be the same other than
	// the line containing "oldpassword" from agent.conf.
	scriptLines := strings.Split(script, "\n")
	sshinitScriptLines := strings.Split(sshinitScript, "\n")
	c.Assert(scriptLines, gc.HasLen, len(sshinitScriptLines))
	for i, line := range scriptLines {
		if strings.Contains(line, "oldpassword") {
			continue
		}
		c.Assert(line, gc.Equals, sshinitScriptLines[i])
	}
}

func (s *clientSuite) TestProvisioningScriptDisablePackageCommands(c *gc.C) {
	apiParams := params.AddMachineParams{
		Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
		HardwareCharacteristics: instance.MustParseHardware("arch=amd64"),
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 1)
	machineId := machines[0].Machine

	provParams := params.ProvisioningScriptParams{
		MachineId: machineId,
		Nonce:     apiParams.Nonce,
	}

	setUpdateBehavior := func(update, upgrade bool) {
		s.State.UpdateEnvironConfig(
			map[string]interface{}{
				"enable-os-upgrade":        upgrade,
				"enable-os-refresh-update": update,
			},
			nil,
			nil,
		)
	}

	// Test enabling package commands
	provParams.DisablePackageCommands = false
	setUpdateBehavior(true, true)
	script, err := s.APIState.Client().ProvisioningScript(provParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(script, jc.Contains, "apt-get update")
	c.Check(script, jc.Contains, "apt-get upgrade")

	// Test disabling package commands
	provParams.DisablePackageCommands = true
	setUpdateBehavior(false, false)
	script, err = s.APIState.Client().ProvisioningScript(provParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(script, gc.Not(jc.Contains), "apt-get update")
	c.Check(script, gc.Not(jc.Contains), "apt-get upgrade")

	// Test client-specified DisablePackageCommands trumps environment
	// config variables.
	provParams.DisablePackageCommands = true
	setUpdateBehavior(true, true)
	script, err = s.APIState.Client().ProvisioningScript(provParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(script, gc.Not(jc.Contains), "apt-get update")
	c.Check(script, gc.Not(jc.Contains), "apt-get upgrade")

	// Test that in the abasence of a client-specified
	// DisablePackageCommands we use what's set in environments.yaml.
	provParams.DisablePackageCommands = false
	setUpdateBehavior(false, false)
	//provParams.UpdateBehavior = &params.UpdateBehavior{false, false}
	script, err = s.APIState.Client().ProvisioningScript(provParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(script, gc.Not(jc.Contains), "apt-get update")
	c.Check(script, gc.Not(jc.Contains), "apt-get upgrade")
}

func (s *clientSuite) TestClientSpecializeStoreOnDeployServiceSetCharmAndAddCharm(c *gc.C) {
	store := s.makeMockCharmStore()

	attrs := map[string]interface{}{"charm-store-auth": "token=value",
		"test-mode": true}
	err := s.State.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	curl, _ := addCharm(c, "dummy")
	err = s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{}, "",
	)
	c.Assert(err, jc.ErrorIsNil)

	// check that the store's auth attributes were set
	c.Assert(store.AuthAttrs(), gc.Equals, "token=value")
	c.Assert(store.TestMode(), jc.IsTrue)

	store.SetAuthAttrs("")

	curl, _ = addCharm(c, "wordpress")
	err = s.APIState.Client().ServiceSetCharm(
		"service", curl.String(), false,
	)

	// check that the store's auth attributes were set
	c.Assert(store.AuthAttrs(), gc.Equals, "token=value")

	curl, _ = addCharm(c, "riak")
	err = s.APIState.Client().AddCharm(curl)

	// check that the store's auth attributes were set
	c.Assert(store.AuthAttrs(), gc.Equals, "token=value")
}

func (s *clientSuite) TestAddCharm(c *gc.C) {
	s.makeMockCharmStore()

	var blobs blobs
	s.PatchValue(client.StateStorage, func(st *state.State) state.Storage {
		storage := st.Storage()
		return &recordingStorage{Storage: storage, blobs: &blobs}
	})

	client := s.APIState.Client()
	// First test the sanity checks.
	err := client.AddCharm(&charm.URL{Name: "nonsense"})
	c.Assert(err, gc.ErrorMatches, `charm URL has invalid schema: ":nonsense-0"`)
	err = client.AddCharm(charm.MustParseURL("local:precise/dummy"))
	c.Assert(err, gc.ErrorMatches, "only charm store charm URLs are supported, with cs: schema")
	err = client.AddCharm(charm.MustParseURL("cs:precise/wordpress"))
	c.Assert(err, gc.ErrorMatches, "charm URL must include revision")

	// Add a charm, without uploading it to storage, to
	// check that AddCharm does not try to do it.
	charmDir := testcharms.Repo.CharmDir("dummy")
	ident := fmt.Sprintf("%s-%d", charmDir.Meta().Name, charmDir.Revision())
	curl := charm.MustParseURL("cs:quantal/" + ident)
	sch, err := s.State.AddCharm(charmDir, curl, "", ident+"-sha256")
	c.Assert(err, jc.ErrorIsNil)

	// AddCharm should see the charm in state and not upload it.
	err = client.AddCharm(sch.URL())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(blobs.m, gc.HasLen, 0)

	// Now try adding another charm completely.
	curl, _ = addCharm(c, "wordpress")
	err = client.AddCharm(curl)
	c.Assert(err, jc.ErrorIsNil)

	// Verify it's in state and it got uploaded.
	storage := s.State.Storage()
	sch, err = s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUploaded(c, storage, sch.StoragePath(), sch.BundleSha256())
}

var resolveCharmCases = []struct {
	schema, defaultSeries, charmName string
	parseErr                         string
	resolveErr                       string
}{
	{"cs", "precise", "wordpress", "", ""},
	{"cs", "trusty", "wordpress", "", ""},
	{"cs", "", "wordpress", "", `charm url series is not resolved`},
	{"cs", "trusty", "", `charm URL has invalid charm name: "cs:"`, ""},
	{"local", "trusty", "wordpress", "", `only charm store charm references are supported, with cs: schema`},
	{"cs", "precise", "hl3", "", ""},
	{"cs", "trusty", "hl3", "", ""},
	{"cs", "", "hl3", "", `charm url series is not resolved`},
}

func (s *clientSuite) TestResolveCharm(c *gc.C) {
	store := s.makeMockCharmStore()

	for i, test := range resolveCharmCases {
		c.Logf("test %d: %#v", i, test)
		// Mock charm store will use this to resolve a charm reference.
		store.SetDefaultSeries(test.defaultSeries)

		client := s.APIState.Client()
		ref, err := charm.ParseReference(fmt.Sprintf("%s:%s", test.schema, test.charmName))
		if test.parseErr == "" {
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
		} else {
			c.Assert(err, gc.NotNil)
			c.Check(err, gc.ErrorMatches, test.parseErr)
			continue
		}
		c.Check(ref.String(), gc.Equals, fmt.Sprintf("%s:%s", test.schema, test.charmName))

		curl, err := client.ResolveCharm(ref)
		if err == nil {
			c.Assert(curl, gc.NotNil)
			// Only cs: schema should make it through here
			c.Check(curl.String(), gc.Equals, fmt.Sprintf("cs:%s/%s", test.defaultSeries, test.charmName))
			c.Check(test.resolveErr, gc.Equals, "")
		} else {
			c.Check(curl, gc.IsNil)
			c.Check(err, gc.ErrorMatches, test.resolveErr)
		}
	}
}

type blobs struct {
	sync.Mutex
	m map[string]bool // maps path to added (true), or deleted (false)
}

// Add adds a path to the list of known paths.
func (b *blobs) Add(path string) {
	b.Lock()
	defer b.Unlock()
	b.check()
	b.m[path] = true
}

// Remove marks a path as deleted, even if it was not previously Added.
func (b *blobs) Remove(path string) {
	b.Lock()
	defer b.Unlock()
	b.check()
	b.m[path] = false
}

func (b *blobs) check() {
	if b.m == nil {
		b.m = make(map[string]bool)
	}
}

type recordingStorage struct {
	state.Storage
	putBarrier *sync.WaitGroup
	blobs      *blobs
}

func (s *recordingStorage) Put(path string, r io.Reader, size int64) error {
	if s.putBarrier != nil {
		// This goroutine has gotten to Put() so mark it Done() and
		// wait for the other goroutines to get to this point.
		s.putBarrier.Done()
		s.putBarrier.Wait()
	}
	if err := s.Storage.Put(path, r, size); err != nil {
		return errors.Trace(err)
	}
	s.blobs.Add(path)
	return nil
}

func (s *recordingStorage) Remove(path string) error {
	if err := s.Storage.Remove(path); err != nil {
		return errors.Trace(err)
	}
	s.blobs.Remove(path)
	return nil
}

func (s *clientSuite) TestAddCharmConcurrently(c *gc.C) {
	s.makeMockCharmStore()

	var putBarrier sync.WaitGroup
	var blobs blobs
	s.PatchValue(client.StateStorage, func(st *state.State) state.Storage {
		storage := st.Storage()
		return &recordingStorage{Storage: storage, blobs: &blobs, putBarrier: &putBarrier}
	})

	client := s.APIState.Client()
	curl, _ := addCharm(c, "wordpress")

	// Try adding the same charm concurrently from multiple goroutines
	// to test no "duplicate key errors" are reported (see lp bug
	// #1067979) and also at the end only one charm document is
	// created.

	var wg sync.WaitGroup
	// We don't add them 1-by-1 because that would allow each goroutine to
	// finish separately without actually synchronizing between them
	putBarrier.Add(10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			c.Assert(client.AddCharm(curl), gc.IsNil, gc.Commentf("goroutine %d", index))
			sch, err := s.State.Charm(curl)
			c.Assert(err, gc.IsNil, gc.Commentf("goroutine %d", index))
			c.Assert(sch.URL(), jc.DeepEquals, curl, gc.Commentf("goroutine %d", index))
		}(i)
	}
	wg.Wait()

	blobs.Lock()

	c.Assert(blobs.m, gc.HasLen, 10)

	// Verify there is only a single uploaded charm remains and it
	// contains the correct data.
	sch, err := s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	storagePath := sch.StoragePath()
	c.Assert(blobs.m[storagePath], jc.IsTrue)
	for path, exists := range blobs.m {
		if path != storagePath {
			c.Assert(exists, jc.IsFalse)
		}
	}

	storage := s.State.Storage()
	s.assertUploaded(c, storage, sch.StoragePath(), sch.BundleSha256())
}

func (s *clientSuite) TestAddCharmOverwritesPlaceholders(c *gc.C) {
	s.makeMockCharmStore()

	client := s.APIState.Client()
	curl, _ := addCharm(c, "wordpress")

	// Add a placeholder with the same charm URL.
	err := s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.Charm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Now try to add the charm, which will convert the placeholder to
	// a pending charm.
	err = client.AddCharm(curl)
	c.Assert(err, jc.ErrorIsNil)

	// Make sure the document's flags were reset as expected.
	sch, err := s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), jc.DeepEquals, curl)
	c.Assert(sch.IsPlaceholder(), jc.IsFalse)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
}

func (s *clientSuite) assertUploaded(c *gc.C, storage state.Storage, storagePath, expectedSHA256 string) {
	reader, _, err := storage.Get(storagePath)
	c.Assert(err, jc.ErrorIsNil)
	defer reader.Close()
	downloadedSHA256, _, err := utils.ReadSHA256(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(downloadedSHA256, gc.Equals, expectedSHA256)
}

func (s *clientSuite) TestRetryProvisioning(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetStatus(state.StatusError, "error", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.APIState.Client().RetryProvisioning(machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	status, info, data, err := machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusError)
	c.Assert(info, gc.Equals, "error")
	c.Assert(data["transient"], jc.IsTrue)
}

func (s *clientSuite) TestAPIHostPorts(c *gc.C) {
	server1Addresses := []network.Address{{
		Value: "server-1",
		Type:  network.HostName,
		Scope: network.ScopePublic,
	}, {
		Value:       "10.0.0.1",
		Type:        network.IPv4Address,
		NetworkName: "internal",
		Scope:       network.ScopeCloudLocal,
	}}
	server2Addresses := []network.Address{{
		Value:       "::1",
		Type:        network.IPv6Address,
		NetworkName: "loopback",
		Scope:       network.ScopeMachineLocal,
	}}
	stateAPIHostPorts := [][]network.HostPort{
		network.AddressesWithPort(server1Addresses, 123),
		network.AddressesWithPort(server2Addresses, 456),
	}

	err := s.State.SetAPIHostPorts(stateAPIHostPorts)
	c.Assert(err, jc.ErrorIsNil)
	apiHostPorts, err := s.APIState.Client().APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiHostPorts, gc.DeepEquals, stateAPIHostPorts)
}

func (s *clientSuite) TestClientAgentVersion(c *gc.C) {
	current := version.MustParse("1.2.0")
	s.PatchValue(&version.Current.Number, current)
	result, err := s.APIState.Client().AgentVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, current)
}

func (s *clientSuite) TestMachineJobFromParams(c *gc.C) {
	var tests = []struct {
		name multiwatcher.MachineJob
		want state.MachineJob
		err  string
	}{{
		name: multiwatcher.JobHostUnits,
		want: state.JobHostUnits,
	}, {
		name: multiwatcher.JobManageEnviron,
		want: state.JobManageEnviron,
	}, {
		name: multiwatcher.JobManageNetworking,
		want: state.JobManageNetworking,
	}, {
		name: multiwatcher.JobManageStateDeprecated,
		want: state.JobManageStateDeprecated,
	}, {
		name: "invalid",
		want: -1,
		err:  `invalid machine job "invalid"`,
	}}
	for _, test := range tests {
		got, err := client.MachineJobFromParams(test.name)
		if err != nil {
			c.Check(err, gc.ErrorMatches, test.err)
		}
		c.Check(got, gc.Equals, test.want)
	}
}

func (s *serverSuite) TestBlockOperationNil(c *gc.C) {
	c.Assert(client.BlockOperation(false, nil), jc.ErrorIsNil)
}

func (s *serverSuite) TestBlockOperationError(c *gc.C) {
	c.Assert(client.BlockOperation(true, nil), gc.Equals, common.ErrOperationBlocked)
}

func (s *serverSuite) TestIgnoreBlockOperationOnServerError(c *gc.C) {
	err := errors.New("My test error")
	expectedErr := common.ServerError(err)
	c.Assert(client.BlockOperation(false, err), gc.DeepEquals, expectedErr)
	c.Assert(client.BlockOperation(true, err), gc.DeepEquals, expectedErr)
}

// blockRemoveObject blocks remove object operations -
// setting block-remove-object to true.
// Asserts that no errors were encountered.
func (s *clientSuite) blockRemoveObject(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"block-remove-object": true}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestBlockServiceDestroy(c *gc.C) {
	s.AddTestingService(c, "dummy-service", s.AddTestingCharm(c, "dummy"))
	// block remove-objects
	s.blockRemoveObject(c)

	for i, t := range serviceDestroyTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.APIState.Client().ServiceDestroy(t.service)
		c.Assert(err, gc.ErrorMatches, common.ErrOperationBlocked.Error())
		// Tests may have invalid service names.
		service, err := s.State.Service(t.service)
		if err == nil {
			// For valid service names, check that service is alive :-)
			assertLife(c, service, state.Alive)
		}
	}
}

func (s *clientSuite) assertDestroyMachineSuccess(c *gc.C, u *state.Unit, m0, m1, m2 *state.Machine) {
	err := s.APIState.Client().DestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the environment; machine 1 has unit "wordpress/0" assigned`)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Alive)
	assertLife(c, m2, state.Dying)

	err = u.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().DestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the environment`)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Dying)
	assertLife(c, m2, state.Dying)
}

func (s *clientSuite) TestBlockDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)
	// block remove-objects
	s.blockRemoveObject(c)
	err := s.APIState.Client().DestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, common.ErrOperationBlocked.Error())
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Alive)
	assertLife(c, u, state.Alive)
	assertLife(c, m2, state.Alive)
}

func (s *clientSuite) assertForceDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)

	err := s.APIState.Client().ForceDestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the environment`)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Alive)
	assertLife(c, m2, state.Alive)
	assertLife(c, u, state.Alive)

	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Dead)
	assertLife(c, m2, state.Dead)
	assertRemoved(c, u)
}

func (s *clientSuite) TestForceBlockDestroyMachines(c *gc.C) {
	// block remove-objects will have no effect here
	s.blockRemoveObject(c)
	s.assertForceDestroyMachines(c)
}

func (s *clientSuite) assertDestroyPrincipalUnits(c *gc.C, units []*state.Unit) {
	// Destroy 2 of them; check they become Dying.
	err := s.APIState.Client().DestroyServiceUnits("wordpress/0", "wordpress/1")
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, units[0], state.Dying)
	assertLife(c, units[1], state.Dying)

	// Try to destroy an Alive one and a Dying one; check
	// it destroys the Alive one and ignores the Dying one.
	err = s.APIState.Client().DestroyServiceUnits("wordpress/2", "wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, units[2], state.Dying)

	// Try to destroy an Alive one along with a nonexistent one; check that
	// the valid instruction is followed but the invalid one is warned about.
	err = s.APIState.Client().DestroyServiceUnits("boojum/123", "wordpress/3")
	c.Assert(err, gc.ErrorMatches, `some units were not destroyed: unit "boojum/123" does not exist`)
	assertLife(c, units[3], state.Dying)

	// Make one Dead, and destroy an Alive one alongside it; check no errors.
	wp0, err := s.State.Unit("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	err = wp0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().DestroyServiceUnits("wordpress/0", "wordpress/4")
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, units[0], state.Dead)
	assertLife(c, units[4], state.Dying)
}

func (s *clientSuite) TestBlockDestroyPrincipalUnits(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	units := make([]*state.Unit, 5)
	for i := range units {
		unit, err := wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = unit.SetStatus(state.StatusStarted, "", nil)
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
	}
	// block remove-objects
	s.blockRemoveObject(c)
	err := s.APIState.Client().DestroyServiceUnits("wordpress/0", "wordpress/1")
	c.Assert(err, gc.ErrorMatches, common.ErrOperationBlocked.Error())
	assertLife(c, units[0], state.Alive)
	assertLife(c, units[1], state.Alive)
	assertLife(c, units[2], state.Alive)
	assertLife(c, units[3], state.Alive)
	assertLife(c, units[4], state.Alive)
}

func (s *clientSuite) assertDestroySubordinateUnits(c *gc.C, wordpress0, logging0 *state.Unit) {
	// Try to destroy the principal and the subordinate together; check it warns
	// about the subordinate, but destroys the one it can. (The principal unit
	// agent will be resposible for destroying the subordinate.)
	err := s.APIState.Client().DestroyServiceUnits("wordpress/0", "logging/0")
	c.Assert(err, gc.ErrorMatches, `some units were not destroyed: unit "logging/0" is a subordinate`)
	assertLife(c, wordpress0, state.Dying)
	assertLife(c, logging0, state.Alive)
}

func (s *clientSuite) TestBlockDestroySubordinateUnits(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	// block remove-objects
	s.blockRemoveObject(c)
	// Try to destroy the subordinate alone; check it fails.
	err = s.APIState.Client().DestroyServiceUnits("logging/0")
	c.Assert(err, gc.ErrorMatches, common.ErrOperationBlocked.Error())
	assertLife(c, rel, state.Alive)
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)

	err = s.APIState.Client().DestroyServiceUnits("wordpress/0", "logging/0")
	c.Assert(err, gc.ErrorMatches, common.ErrOperationBlocked.Error())
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)
	assertLife(c, rel, state.Alive)
}

func (s *clientSuite) TestBlockDestroyRelation(c *gc.C) {
	endpoints := []string{"wordpress", "mysql"}
	relation := s.setupRelationScenario(c, endpoints)
	// block remove-objects
	s.blockRemoveObject(c)
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, common.ErrOperationBlocked.Error())
	assertLife(c, relation, state.Alive)
}

func (s *clientSuite) TestBlockDestroyNoRelation(c *gc.C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress", "mysql"}

	// block remove-objects
	s.blockRemoveObject(c)
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, common.ErrOperationBlocked.Error())
}

func (s *clientSuite) TestBlockDestroyingNonExistentRelation(c *gc.C) {
	s.setUpScenario(c)
	s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))
	endpoints := []string{"riak", "wordpress"}
	// block remove-objects
	s.blockRemoveObject(c)
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, common.ErrOperationBlocked.Error())
}

func (s *clientSuite) TestBlockDestroyingWithOnlyOneEndpoint(c *gc.C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress"}
	// block remove-objects
	s.blockRemoveObject(c)
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, common.ErrOperationBlocked.Error())
}

func (s *clientSuite) TestBlockDestroyingPeerRelation(c *gc.C) {
	s.setUpScenario(c)
	s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))
	endpoints := []string{"riak:ring"}

	// block remove-objects
	s.blockRemoveObject(c)
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, common.ErrOperationBlocked.Error())
}

func (s *clientSuite) TestBlockDestroyingAlreadyDestroyedRelation(c *gc.C) {
	s.setUpScenario(c)

	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := []string{"wordpress", "mysql"}
	err = s.APIState.Client().DestroyRelation(endpoints...)
	// Show that the relation was removed.
	c.Assert(rel.Refresh(), jc.Satisfies, errors.IsNotFound)

	// block remove-objects
	s.blockRemoveObject(c)
	// And try to destroy it again.
	err = s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, common.ErrOperationBlocked.Error())
}
