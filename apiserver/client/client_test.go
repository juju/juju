// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/presence"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
)

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

func (s *serverSuite) TestModelUsersInfo(c *gc.C) {
	testAdmin := s.AdminUserTag(c)
	owner, err := s.State.ModelUser(testAdmin)
	c.Assert(err, jc.ErrorIsNil)

	localUser1 := s.makeLocalModelUser(c, "ralphdoe", "Ralph Doe")
	localUser2 := s.makeLocalModelUser(c, "samsmith", "Sam Smith")
	remoteUser1 := s.Factory.MakeModelUser(c, &factory.ModelUserParams{User: "bobjohns@ubuntuone", DisplayName: "Bob Johns"})
	remoteUser2 := s.Factory.MakeModelUser(c, &factory.ModelUserParams{User: "nicshaw@idprovider", DisplayName: "Nic Shaw"})

	results, err := s.client.ModelUserInfo()
	c.Assert(err, jc.ErrorIsNil)
	var expected params.ModelUserInfoResults
	for _, r := range []struct {
		user *state.ModelUser
		info *params.ModelUserInfo
	}{
		{
			owner,
			&params.ModelUserInfo{
				UserName:    owner.UserName(),
				DisplayName: owner.DisplayName(),
			},
		}, {
			localUser1,
			&params.ModelUserInfo{
				UserName:    "ralphdoe@local",
				DisplayName: "Ralph Doe",
			},
		}, {
			localUser2,
			&params.ModelUserInfo{
				UserName:    "samsmith@local",
				DisplayName: "Sam Smith",
			},
		}, {
			remoteUser1,
			&params.ModelUserInfo{
				UserName:    "bobjohns@ubuntuone",
				DisplayName: "Bob Johns",
			},
		}, {
			remoteUser2,
			&params.ModelUserInfo{
				UserName:    "nicshaw@idprovider",
				DisplayName: "Nic Shaw",
			},
		},
	} {
		r.info.CreatedBy = owner.UserName()
		r.info.DateCreated = r.user.DateCreated()
		r.info.LastConnection = lastConnPointer(c, r.user)
		expected.Results = append(expected.Results, params.ModelUserInfoResult{Result: r.info})
	}

	sort.Sort(ByUserName(expected.Results))
	sort.Sort(ByUserName(results.Results))
	c.Assert(results, jc.DeepEquals, expected)
}

func lastConnPointer(c *gc.C, modelUser *state.ModelUser) *time.Time {
	lastConn, err := modelUser.LastConnection()
	if err != nil {
		if state.IsNeverConnectedError(err) {
			return nil
		}
		c.Fatal(err)
	}
	return &lastConn
}

// ByUserName implements sort.Interface for []params.ModelUserInfoResult based on
// the UserName field.
type ByUserName []params.ModelUserInfoResult

func (a ByUserName) Len() int           { return len(a) }
func (a ByUserName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByUserName) Less(i, j int) bool { return a[i].Result.UserName < a[j].Result.UserName }

func (s *serverSuite) makeLocalModelUser(c *gc.C, username, displayname string) *state.ModelUser {
	// factory.MakeUser will create an ModelUser for a local user by defalut
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: username, DisplayName: displayname})
	modelUser, err := s.State.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	return modelUser
}

func (s *serverSuite) TestShareModelAddMissingLocalFails(c *gc.C) {
	args := params.ModifyModelUsers{
		Changes: []params.ModifyModelUser{{
			UserTag: names.NewLocalUserTag("foobar").String(),
			Action:  params.AddModelUser,
		}}}

	result, err := s.client.ShareModel(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `could not share model: user "foobar" does not exist locally: user "foobar" not found`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *serverSuite) TestUnshareModel(c *gc.C) {
	user := s.Factory.MakeModelUser(c, nil)
	_, err := s.State.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)

	args := params.ModifyModelUsers{
		Changes: []params.ModifyModelUser{{
			UserTag: user.UserTag().String(),
			Action:  params.RemoveModelUser,
		}}}

	result, err := s.client.ShareModel(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	_, err = s.State.ModelUser(user.UserTag())
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *serverSuite) TestUnshareModelMissingUser(c *gc.C) {
	user := names.NewUserTag("bob")
	args := params.ModifyModelUsers{
		Changes: []params.ModifyModelUser{{
			UserTag: user.String(),
			Action:  params.RemoveModelUser,
		}}}

	result, err := s.client.ShareModel(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.ErrorMatches, `could not unshare model: env user "bob@local" does not exist: transaction aborted`)

	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.NotNil)

	_, err = s.State.ModelUser(user)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *serverSuite) TestShareModelAddLocalUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", NoModelUser: true})
	args := params.ModifyModelUsers{
		Changes: []params.ModifyModelUser{{
			UserTag: user.Tag().String(),
			Action:  params.AddModelUser,
		}}}

	result, err := s.client.ShareModel(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	modelUser, err := s.State.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName(), gc.Equals, user.UserTag().Canonical())
	c.Assert(modelUser.CreatedBy(), gc.Equals, "admin@local")
	lastConn, err := modelUser.LastConnection()
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(lastConn, gc.Equals, time.Time{})
}

func (s *serverSuite) TestShareModelAddRemoteUser(c *gc.C) {
	user := names.NewUserTag("foobar@ubuntuone")
	args := params.ModifyModelUsers{
		Changes: []params.ModifyModelUser{{
			UserTag: user.String(),
			Action:  params.AddModelUser,
		}}}

	result, err := s.client.ShareModel(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	modelUser, err := s.State.ModelUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName(), gc.Equals, user.Canonical())
	c.Assert(modelUser.CreatedBy(), gc.Equals, "admin@local")
	lastConn, err := modelUser.LastConnection()
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(lastConn.IsZero(), jc.IsTrue)
}

func (s *serverSuite) TestShareModelAddUserTwice(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar"})
	args := params.ModifyModelUsers{
		Changes: []params.ModifyModelUser{{
			UserTag: user.Tag().String(),
			Action:  params.AddModelUser,
		}}}

	_, err := s.client.ShareModel(args)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.ShareModel(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.ErrorMatches, "could not share model: model user \"foobar@local\" already exists")
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "could not share model: model user \"foobar@local\" already exists")
	c.Assert(result.Results[0].Error.Code, gc.Matches, params.CodeAlreadyExists)

	modelUser, err := s.State.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName(), gc.Equals, user.UserTag().Canonical())
}

func (s *serverSuite) TestShareModelInvalidTags(c *gc.C) {
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
		errPart := `could not share model: "` + regexp.QuoteMeta(testParam.tag) + `" is not a valid `

		if testParam.validTag {

			// The string is a valid tag, but not a user tag.
			expectedErr = errPart + `user tag`
		} else {

			// The string is not a valid tag of any kind.
			expectedErr = errPart + `tag`
		}

		args := params.ModifyModelUsers{
			Changes: []params.ModifyModelUser{{
				UserTag: testParam.tag,
				Action:  params.AddModelUser,
			}}}

		_, err := s.client.ShareModel(args)
		result, err := s.client.ShareModel(args)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
		c.Assert(result.Results, gc.HasLen, 1)
		c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedErr)
	}
}

func (s *serverSuite) TestShareModelZeroArgs(c *gc.C) {
	args := params.ModifyModelUsers{Changes: []params.ModifyModelUser{{}}}

	_, err := s.client.ShareModel(args)
	result, err := s.client.ShareModel(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `could not share model: "" is not a valid tag`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *serverSuite) TestShareModelInvalidAction(c *gc.C) {
	var dance params.ModelAction = "dance"
	args := params.ModifyModelUsers{
		Changes: []params.ModifyModelUser{{
			UserTag: "user-user@local",
			Action:  dance,
		}}}

	_, err := s.client.ShareModel(args)
	result, err := s.client.ShareModel(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `unknown action "dance"`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *serverSuite) TestSetEnvironAgentVersion(c *gc.C) {
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(err, jc.ErrorIsNil)

	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, found := envConfig.AllAttrs()["agent-version"]
	c.Assert(found, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, "9.8.7")
}

type mockEnviron struct {
	environs.Environ
	allInstancesCalled bool
	err                error
}

func (m *mockEnviron) AllInstances() ([]instance.Instance, error) {
	m.allInstancesCalled = true
	return nil, m.err
}

func (s *serverSuite) assertCheckProviderAPI(c *gc.C, envError error, expectErr string) {
	env := &mockEnviron{err: envError}
	s.PatchValue(client.GetEnvironment, func(cfg *config.Config) (environs.Environ, error) {
		return env, nil
	})
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(env.allInstancesCalled, jc.IsTrue)
	if expectErr != "" {
		c.Assert(err, gc.ErrorMatches, expectErr)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *serverSuite) TestCheckProviderAPISuccess(c *gc.C) {
	s.assertCheckProviderAPI(c, nil, "")
	s.assertCheckProviderAPI(c, environs.ErrPartialInstances, "")
	s.assertCheckProviderAPI(c, environs.ErrNoInstances, "")
}

func (s *serverSuite) TestCheckProviderAPIFail(c *gc.C) {
	s.assertCheckProviderAPI(c, fmt.Errorf("instances error"), "cannot make API call to provider: instances error")
}

func (s *serverSuite) assertSetEnvironAgentVersion(c *gc.C) {
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetModelAgentVersion(args)
	c.Assert(err, jc.ErrorIsNil)
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, found := envConfig.AllAttrs()["agent-version"]
	c.Assert(found, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, "9.8.7")
}

func (s *serverSuite) assertSetEnvironAgentVersionBlocked(c *gc.C, msg string) {
	args := params.SetModelAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetModelAgentVersion(args)
	s.AssertBlocked(c, err, msg)
}

func (s *serverSuite) TestBlockDestroySetEnvironAgentVersion(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroySetEnvironAgentVersion")
	s.assertSetEnvironAgentVersion(c)
}

func (s *serverSuite) TestBlockRemoveSetEnvironAgentVersion(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveSetEnvironAgentVersion")
	s.assertSetEnvironAgentVersion(c)
}

func (s *serverSuite) TestBlockChangesSetEnvironAgentVersion(c *gc.C) {
	s.BlockAllChanges(c, "TestBlockChangesSetEnvironAgentVersion")
	s.assertSetEnvironAgentVersionBlocked(c, "TestBlockChangesSetEnvironAgentVersion")
}

func (s *serverSuite) TestAbortCurrentUpgrade(c *gc.C) {
	// Create a provisioned controller.
	machine, err := s.State.AddMachine("series", state.JobManageModel)
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

func (s *serverSuite) assertAbortCurrentUpgradeBlocked(c *gc.C, msg string) {
	err := s.client.AbortCurrentUpgrade()
	s.AssertBlocked(c, err, msg)
}

func (s *serverSuite) assertAbortCurrentUpgrade(c *gc.C) {
	err := s.client.AbortCurrentUpgrade()
	c.Assert(err, jc.ErrorIsNil)
	isUpgrading, err := s.State.IsUpgrading()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isUpgrading, jc.IsFalse)
}

func (s *serverSuite) setupAbortCurrentUpgradeBlocked(c *gc.C) {
	// Create a provisioned controller.
	machine, err := s.State.AddMachine("series", state.JobManageModel)
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
}

func (s *serverSuite) TestBlockDestroyAbortCurrentUpgrade(c *gc.C) {
	s.setupAbortCurrentUpgradeBlocked(c)
	s.BlockDestroyModel(c, "TestBlockDestroyAbortCurrentUpgrade")
	s.assertAbortCurrentUpgrade(c)
}

func (s *serverSuite) TestBlockRemoveAbortCurrentUpgrade(c *gc.C) {
	s.setupAbortCurrentUpgradeBlocked(c)
	s.BlockRemoveObject(c, "TestBlockRemoveAbortCurrentUpgrade")
	s.assertAbortCurrentUpgrade(c)
}

func (s *serverSuite) TestBlockChangesAbortCurrentUpgrade(c *gc.C) {
	s.setupAbortCurrentUpgradeBlocked(c)
	s.BlockAllChanges(c, "TestBlockChangesAbortCurrentUpgrade")
	s.assertAbortCurrentUpgradeBlocked(c, "TestBlockChangesAbortCurrentUpgrade")
}

type clientSuite struct {
	baseSuite
}

var _ = gc.Suite(&clientSuite{})

// clearSinceTimes zeros out the updated timestamps inside status
// so we can easily check the results.
func clearSinceTimes(status *params.FullStatus) {
	for serviceId, service := range status.Services {
		for unitId, unit := range service.Units {
			unit.Workload.Since = nil
			unit.UnitAgent.Since = nil
			for id, subord := range unit.Subordinates {
				subord.Workload.Since = nil
				subord.UnitAgent.Since = nil
				unit.Subordinates[id] = subord
			}
			service.Units[unitId] = unit
		}
		service.Status.Since = nil
		status.Services[serviceId] = service
	}
	for id, machine := range status.Machines {
		machine.Agent.Since = nil
		status.Machines[id] = machine
	}
}

func (s *clientSuite) TestClientStatus(c *gc.C) {
	s.setUpScenario(c)
	status, err := s.APIState.Client().Status(nil)
	clearSinceTimes(status)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, jc.DeepEquals, scenarioStatus)
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
					"snapshot": {
						Description: "Take a snapshot of the database.",
						Params: map[string]interface{}{
							"type":        "object",
							"title":       "snapshot",
							"description": "Take a snapshot of the database.",
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
			charm: "wordpress",
			expectedActions: &charm.Actions{ActionSpecs: map[string]charm.ActionSpec{
				"fakeaction": {
					Description: "No description",
					Params: map[string]interface{}{
						"type":        "object",
						"title":       "fakeaction",
						"description": "No description",
						"properties":  map[string]interface{}{},
					},
				},
			}},
			url: "local:quantal/wordpress-3",
		},
		{
			about: "invalid URL",
			charm: "wordpress",
			url:   "not-valid!",
			err:   `URL has invalid charm or bundle name: "not-valid!"`,
		},
		{
			about: "invalid schema",
			charm: "wordpress",
			url:   "not-valid:your-arguments",
			err:   `charm or bundle URL has invalid schema: "not-valid:your-arguments"`,
		},
		{
			about: "unknown charm",
			charm: "wordpress",
			url:   "cs:missing/one-1",
			err:   `charm "cs:missing/one-1" not found \(not found\)`,
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

func (s *clientSuite) TestClientModelInfo(c *gc.C) {
	conf, _ := s.State.ModelConfig()
	info, err := s.APIState.Client().ModelInfo()
	c.Assert(err, jc.ErrorIsNil)
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.DefaultSeries, gc.Equals, config.PreferredSeries(conf))
	c.Assert(info.ProviderType, gc.Equals, conf.Type())
	c.Assert(info.Name, gc.Equals, conf.Name())
	c.Assert(info.UUID, gc.Equals, env.UUID())
	c.Assert(info.ControllerUUID, gc.Equals, env.ControllerUUID())
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
	m0, err := s.State.AddMachine("quantal", state.JobManageModel)
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

func (s *clientSuite) testClientUnitResolved(c *gc.C, retry bool, expectedResolvedMode state.ResolvedMode) {
	// Setup:
	s.setUpScenario(c)
	u, err := s.State.Unit("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetAgentStatus(state.StatusError, "gaaah", nil)
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

func (s *clientSuite) setupResolved(c *gc.C) *state.Unit {
	s.setUpScenario(c)
	u, err := s.State.Unit("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetAgentStatus(state.StatusError, "gaaah", nil)
	c.Assert(err, jc.ErrorIsNil)
	return u
}

func (s *clientSuite) assertResolved(c *gc.C, u *state.Unit) {
	err := s.APIState.Client().Resolved("wordpress/0", true)
	c.Assert(err, jc.ErrorIsNil)
	// Freshen the unit's state.
	err = u.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	// And now the actual test assertions: we set the unit as resolved via
	// the API so it should have a resolved mode set.
	mode := u.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedRetryHooks)
}

func (s *clientSuite) assertResolvedBlocked(c *gc.C, u *state.Unit, msg string) {
	err := s.APIState.Client().Resolved("wordpress/0", true)
	s.AssertBlocked(c, err, msg)
}

func (s *clientSuite) TestBlockDestroyUnitResolved(c *gc.C) {
	u := s.setupResolved(c)
	s.BlockDestroyModel(c, "TestBlockDestroyUnitResolved")
	s.assertResolved(c, u)
}

func (s *clientSuite) TestBlockRemoveUnitResolved(c *gc.C) {
	u := s.setupResolved(c)
	s.BlockRemoveObject(c, "TestBlockRemoveUnitResolved")
	s.assertResolved(c, u)
}

func (s *clientSuite) TestBlockChangeUnitResolved(c *gc.C) {
	u := s.setupResolved(c)
	s.BlockAllChanges(c, "TestBlockChangeUnitResolved")
	s.assertResolvedBlocked(c, u, "TestBlockChangeUnitResolved")
}

type clientRepoSuite struct {
	baseSuite
	testing.CharmStoreSuite
}

var _ = gc.Suite(&clientRepoSuite{})

func (s *clientRepoSuite) SetUpSuite(c *gc.C) {
	s.CharmStoreSuite.SetUpSuite(c)
	s.baseSuite.SetUpSuite(c)

}

func (s *clientRepoSuite) TearDownSuite(c *gc.C) {
	s.CharmStoreSuite.TearDownSuite(c)
	s.baseSuite.TearDownSuite(c)
}

func (s *clientRepoSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.CharmStoreSuite.Session = s.baseSuite.Session
	s.CharmStoreSuite.SetUpTest(c)

	c.Assert(s.APIState, gc.NotNil)
}

func (s *clientRepoSuite) TearDownTest(c *gc.C) {
	s.CharmStoreSuite.TearDownTest(c)
	s.baseSuite.TearDownTest(c)
}

func (s *clientSuite) TestClientWatchAll(c *gc.C) {
	// A very simple end-to-end test, because
	// all the logic is tested elsewhere.
	m, err := s.State.AddMachine("quantal", state.JobManageModel)
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
			ModelUUID:               s.State.ModelUUID(),
			Id:                      m.Id(),
			InstanceId:              "i-0",
			Status:                  multiwatcher.Status("pending"),
			StatusData:              map[string]interface{}{},
			Life:                    multiwatcher.Life("alive"),
			Series:                  "quantal",
			Jobs:                    []multiwatcher.MachineJob{state.JobManageModel.ToParams()},
			Addresses:               []network.Address{},
			HardwareCharacteristics: &instance.HardwareCharacteristics{},
			HasVote:                 false,
			WantsVote:               true,
		},
	}}) {
		c.Logf("got:")
		for _, d := range deltas {
			c.Logf("%#v\n", d.Entity)
		}
	}
}

func (s *clientSuite) TestClientSetModelConstraints(c *gc.C) {
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().SetModelConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := s.State.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) assertSetModelConstraints(c *gc.C) {
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().SetModelConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)
	// Ensure the constraints have been correctly updated.
	obtained, err := s.State.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) assertSetModelConstraintsBlocked(c *gc.C, msg string) {
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().SetModelConstraints(cons)
	s.AssertBlocked(c, err, msg)
}

func (s *clientSuite) TestBlockDestroyClientSetModelConstraints(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyClientSetModelConstraints")
	s.assertSetModelConstraints(c)
}

func (s *clientSuite) TestBlockRemoveClientSetModelConstraints(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveClientSetModelConstraints")
	s.assertSetModelConstraints(c)
}

func (s *clientSuite) TestBlockChangesClientSetModelConstraints(c *gc.C) {
	s.BlockAllChanges(c, "TestBlockChangesClientSetModelConstraints")
	s.assertSetModelConstraintsBlocked(c, "TestBlockChangesClientSetModelConstraints")
}

func (s *clientSuite) TestClientGetModelConstraints(c *gc.C) {
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetModelConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)

	// Check we can get the constraints.
	obtained, err := s.APIState.Client().GetModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientPublicAddressErrors(c *gc.C) {
	s.setUpScenario(c)
	_, err := s.APIState.Client().PublicAddress("wordpress")
	c.Assert(err, gc.ErrorMatches, `unknown unit or machine "wordpress"`)
	_, err = s.APIState.Client().PublicAddress("0")
	c.Assert(err, gc.ErrorMatches, `error fetching address for machine "0": public no address`)
	_, err = s.APIState.Client().PublicAddress("wordpress/0")
	c.Assert(err, gc.ErrorMatches, `error fetching address for unit "wordpress/0": public no address`)
}

func (s *clientSuite) TestClientPublicAddressMachine(c *gc.C) {
	s.setUpScenario(c)
	network.SetPreferIPv6(false)

	// Internally, network.SelectPublicAddress is used; the "most public"
	// address is returned.
	m1, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	cloudLocalAddress := network.NewScopedAddress("cloudlocal", network.ScopeCloudLocal)
	publicAddress := network.NewScopedAddress("public", network.ScopePublic)
	err = m1.SetProviderAddresses(cloudLocalAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PublicAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "cloudlocal")
	err = m1.SetProviderAddresses(cloudLocalAddress, publicAddress)
	addr, err = s.APIState.Client().PublicAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "public")
}

func (s *clientSuite) TestClientPublicAddressUnit(c *gc.C) {
	s.setUpScenario(c)

	m1, err := s.State.Machine("1")
	publicAddress := network.NewScopedAddress("public", network.ScopePublic)
	err = m1.SetProviderAddresses(publicAddress)
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
	c.Assert(err, gc.ErrorMatches, `error fetching address for machine "0": private no address`)
	_, err = s.APIState.Client().PrivateAddress("wordpress/0")
	c.Assert(err, gc.ErrorMatches, `error fetching address for unit "wordpress/0": private no address`)
}

func (s *clientSuite) TestClientPrivateAddress(c *gc.C) {
	s.setUpScenario(c)
	network.SetPreferIPv6(false)

	// Internally, network.SelectInternalAddress is used; the public
	// address if no cloud-local one is available.
	m1, err := s.State.Machine("1")
	c.Assert(err, jc.ErrorIsNil)
	cloudLocalAddress := network.NewScopedAddress("cloudlocal", network.ScopeCloudLocal)
	publicAddress := network.NewScopedAddress("public", network.ScopePublic)
	err = m1.SetProviderAddresses(publicAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PrivateAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "public")
	err = m1.SetProviderAddresses(cloudLocalAddress, publicAddress)
	addr, err = s.APIState.Client().PrivateAddress("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "cloudlocal")
}

func (s *clientSuite) TestClientPrivateAddressUnit(c *gc.C) {
	s.setUpScenario(c)

	m1, err := s.State.Machine("1")
	privateAddress := network.NewScopedAddress("private", network.ScopeCloudLocal)
	err = m1.SetProviderAddresses(privateAddress)
	c.Assert(err, jc.ErrorIsNil)
	addr, err := s.APIState.Client().PrivateAddress("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, gc.Equals, "private")
}

func (s *serverSuite) TestClientModelGet(c *gc.C) {
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.client.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config, gc.DeepEquals, envConfig.AllAttrs())
}

func (s *serverSuite) assertEnvValue(c *gc.C, key string, expected interface{}) {
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	value, found := envConfig.AllAttrs()[key]
	c.Assert(found, jc.IsTrue)
	c.Assert(value, gc.Equals, expected)
}

func (s *serverSuite) assertEnvValueMissing(c *gc.C, key string) {
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found := envConfig.AllAttrs()[key]
	c.Assert(found, jc.IsFalse)
}

func (s *serverSuite) TestClientModelSet(c *gc.C) {
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found := envConfig.AllAttrs()["some-key"]
	c.Assert(found, jc.IsFalse)

	params := params.ModelSet{
		Config: map[string]interface{}{
			"some-key":  "value",
			"other-key": "other value"},
	}
	err = s.client.ModelSet(params)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvValue(c, "some-key", "value")
	s.assertEnvValue(c, "other-key", "other value")
}

func (s *serverSuite) TestClientModelSetImmutable(c *gc.C) {
	// The various immutable config values are tested in
	// environs/config/config_test.go, so just choosing one here.
	params := params.ModelSet{
		Config: map[string]interface{}{"state-port": "1"},
	}
	err := s.client.ModelSet(params)
	c.Check(err, gc.ErrorMatches, `cannot change state-port from .* to 1`)
}

func (s *serverSuite) assertModelSetBlocked(c *gc.C, args map[string]interface{}, msg string) {
	err := s.client.ModelSet(params.ModelSet{args})
	s.AssertBlocked(c, err, msg)
}

func (s *serverSuite) TestBlockChangesClientModelSet(c *gc.C) {
	s.BlockAllChanges(c, "TestBlockChangesClientModelSet")
	args := map[string]interface{}{"some-key": "value"}
	s.assertModelSetBlocked(c, args, "TestBlockChangesClientModelSet")
}

func (s *serverSuite) TestClientModelSetDeprecated(c *gc.C) {
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	url := envConfig.AllAttrs()["agent-metadata-url"]
	c.Assert(url, gc.Equals, "")

	args := params.ModelSet{
		Config: map[string]interface{}{"tools-metadata-url": "value"},
	}
	err = s.client.ModelSet(args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvValue(c, "agent-metadata-url", "value")
	s.assertEnvValue(c, "tools-metadata-url", "value")
}

func (s *serverSuite) TestClientModelSetCannotChangeAgentVersion(c *gc.C) {
	args := params.ModelSet{
		map[string]interface{}{"agent-version": "9.9.9"},
	}
	err := s.client.ModelSet(args)
	c.Assert(err, gc.ErrorMatches, "agent-version cannot be changed")

	// It's okay to pass env back with the same agent-version.
	result, err := s.client.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config["agent-version"], gc.NotNil)
	args.Config["agent-version"] = result.Config["agent-version"]
	err = s.client.ModelSet(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) TestClientModelUnset(c *gc.C) {
	err := s.State.UpdateModelConfig(map[string]interface{}{"abc": 123}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	args := params.ModelUnset{[]string{"abc"}}
	err = s.client.ModelUnset(args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvValueMissing(c, "abc")
}

func (s *serverSuite) TestBlockClientModelUnset(c *gc.C) {
	err := s.State.UpdateModelConfig(map[string]interface{}{"abc": 123}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.BlockAllChanges(c, "TestBlockClientModelUnset")

	args := params.ModelUnset{[]string{"abc"}}
	err = s.client.ModelUnset(args)
	s.AssertBlocked(c, err, "TestBlockClientModelUnset")
}

func (s *serverSuite) TestClientModelUnsetMissing(c *gc.C) {
	// It's okay to unset a non-existent attribute.
	args := params.ModelUnset{[]string{"not_there"}}
	err := s.client.ModelUnset(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) TestClientModelUnsetError(c *gc.C) {
	err := s.State.UpdateModelConfig(map[string]interface{}{"abc": 123}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// "type" may not be removed, and this will cause an error.
	// If any one attribute's removal causes an error, there
	// should be no change.
	args := params.ModelUnset{[]string{"abc", "type"}}
	err = s.client.ModelUnset(args)
	c.Assert(err, gc.ErrorMatches, "type: expected string, got nothing")
	s.assertEnvValue(c, "abc", 123)
}

func (s *clientSuite) TestClientFindTools(c *gc.C) {
	result, err := s.APIState.Client().FindTools(99, -1, "", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, jc.Satisfies, params.IsCodeNotFound)
	toolstesting.UploadToStorage(c, s.DefaultToolsStorage, "released", version.MustParseBinary("2.99.0-precise-amd64"))
	result, err = s.APIState.Client().FindTools(2, 99, "precise", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.List, gc.HasLen, 1)
	c.Assert(result.List[0].Version, gc.Equals, version.MustParseBinary("2.99.0-precise-amd64"))
	url := fmt.Sprintf("https://%s/model/%s/tools/%s",
		s.APIState.Addr(), coretesting.ModelTag.Id(), result.List[0].Version)
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

func (s *clientSuite) assertAddMachines(c *gc.C) {
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

func (s *clientSuite) assertAddMachinesBlocked(c *gc.C, msg string) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		}
	}
	_, err := s.APIState.Client().AddMachines(apiParams)
	s.AssertBlocked(c, err, msg)
}

func (s *clientSuite) TestBlockDestroyClientAddMachinesDefaultSeries(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyClientAddMachinesDefaultSeries")
	s.assertAddMachines(c)
}

func (s *clientSuite) TestBlockRemoveClientAddMachinesDefaultSeries(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveClientAddMachinesDefaultSeries")
	s.assertAddMachines(c)
}

func (s *clientSuite) TestBlockChangesClientAddMachines(c *gc.C) {
	s.BlockAllChanges(c, "TestBlockChangesClientAddMachines")
	s.assertAddMachinesBlocked(c, "TestBlockChangesClientAddMachines")
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

// updateConfig sets config variable with given key to a given value
// Asserts that no errors were encountered.
func (s *baseSuite) updateConfig(c *gc.C, key string, block bool) {
	err := s.State.UpdateModelConfig(map[string]interface{}{key: block}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
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
	apiParams[2].Placement = instance.MustParsePlacement("dummymodel:invalid")
	apiParams[3].Placement = instance.MustParsePlacement("dummymodel:valid")
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
	// This will cause a add-machine to fail due to an unsupported container.
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
	addrs := network.NewAddresses("1.2.3.4")
	hc := instance.MustParseHardware("mem=4G")
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs:       []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
			InstanceId: instance.Id(fmt.Sprintf("1234-%d", i)),
			Nonce:      "foo",
			HardwareCharacteristics: hc,
			Addrs: params.FromNetworkAddresses(addrs),
		}
	}
	// This will cause the last add-machine to fail.
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
	err := s.APIState.APICall("Client", 1, "", "AddMachines", args, &results)
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
	icfg, err := client.InstanceConfig(s.State, machineId, apiParams.Nonce, "")
	c.Assert(err, jc.ErrorIsNil)
	provisioningScript, err := manual.ProvisioningScript(icfg)
	c.Assert(err, jc.ErrorIsNil)
	// ProvisioningScript internally calls MachineConfig,
	// which allocates a new, random password. Everything
	// about the scripts should be the same other than
	// the line containing "oldpassword" from agent.conf.
	scriptLines := strings.Split(script, "\n")
	provisioningScriptLines := strings.Split(provisioningScript, "\n")
	c.Assert(scriptLines, gc.HasLen, len(provisioningScriptLines))
	for i, line := range scriptLines {
		if strings.Contains(line, "oldpassword") {
			continue
		}
		c.Assert(line, gc.Equals, provisioningScriptLines[i])
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
		s.State.UpdateModelConfig(
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
	// DisablePackageCommands we use what's set in environment config.
	provParams.DisablePackageCommands = false
	setUpdateBehavior(false, false)
	//provParams.UpdateBehavior = &params.UpdateBehavior{false, false}
	script, err = s.APIState.Client().ProvisioningScript(provParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(script, gc.Not(jc.Contains), "apt-get update")
	c.Check(script, gc.Not(jc.Contains), "apt-get upgrade")
}

var resolveCharmTests = []struct {
	about      string
	url        string
	resolved   string
	parseErr   string
	resolveErr string
}{{
	about:    "wordpress resolved",
	url:      "cs:wordpress",
	resolved: "cs:trusty/wordpress",
}, {
	about:    "mysql resolved",
	url:      "cs:mysql",
	resolved: "cs:precise/mysql",
}, {
	about:    "riak resolved",
	url:      "cs:riak",
	resolved: "cs:trusty/riak",
}, {
	about:    "fully qualified char reference",
	url:      "cs:utopic/riak-5",
	resolved: "cs:utopic/riak-5",
}, {
	about:    "charm with series and no revision",
	url:      "cs:precise/wordpress",
	resolved: "cs:precise/wordpress",
}, {
	about:      "fully qualified reference not found",
	url:        "cs:utopic/riak-42",
	resolveErr: `cannot resolve URL "cs:utopic/riak-42": charm not found`,
}, {
	about:      "reference not found",
	url:        "cs:no-such",
	resolveErr: `cannot resolve URL "cs:no-such": charm or bundle not found`,
}, {
	about:    "invalid charm name",
	url:      "cs:",
	parseErr: `URL has invalid charm or bundle name: "cs:"`,
}, {
	about:      "local charm",
	url:        "local:wordpress",
	resolveErr: `only charm store charm references are supported, with cs: schema`,
}}

func (s *clientRepoSuite) TestResolveCharm(c *gc.C) {
	// Add some charms to be resolved later.
	for _, url := range []string{
		"precise/wordpress-1",
		"trusty/wordpress-2",
		"precise/mysql-3",
		"trusty/riak-4",
		"utopic/riak-5",
	} {
		s.UploadCharm(c, url, "wordpress")
	}

	// Run the tests.
	for i, test := range resolveCharmTests {
		c.Logf("test %d: %s", i, test.about)

		client := s.APIState.Client()
		ref, err := charm.ParseURL(test.url)
		if test.parseErr == "" {
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
		} else {
			c.Assert(err, gc.NotNil)
			c.Check(err, gc.ErrorMatches, test.parseErr)
			continue
		}

		curl, err := client.ResolveCharm(ref)
		if test.resolveErr == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Check(curl.String(), gc.Equals, test.resolved)
			continue
		}
		c.Check(err, gc.ErrorMatches, test.resolveErr)
		c.Check(curl, gc.IsNil)
	}
}

func (s *clientSuite) TestRetryProvisioning(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetStatus(state.StatusError, "error", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.APIState.Client().RetryProvisioning(machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusError)
	c.Assert(statusInfo.Message, gc.Equals, "error")
	c.Assert(statusInfo.Data["transient"], jc.IsTrue)
}

func (s *clientSuite) setupRetryProvisioning(c *gc.C) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetStatus(state.StatusError, "error", nil)
	c.Assert(err, jc.ErrorIsNil)
	return machine
}

func (s *clientSuite) assertRetryProvisioning(c *gc.C, machine *state.Machine) {
	_, err := s.APIState.Client().RetryProvisioning(machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err := machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusError)
	c.Assert(statusInfo.Message, gc.Equals, "error")
	c.Assert(statusInfo.Data["transient"], jc.IsTrue)
}

func (s *clientSuite) assertRetryProvisioningBlocked(c *gc.C, machine *state.Machine, msg string) {
	_, err := s.APIState.Client().RetryProvisioning(machine.Tag().(names.MachineTag))
	s.AssertBlocked(c, err, msg)
}

func (s *clientSuite) TestBlockDestroyRetryProvisioning(c *gc.C) {
	m := s.setupRetryProvisioning(c)
	s.BlockDestroyModel(c, "TestBlockDestroyRetryProvisioning")
	s.assertRetryProvisioning(c, m)
}

func (s *clientSuite) TestBlockRemoveRetryProvisioning(c *gc.C) {
	m := s.setupRetryProvisioning(c)
	s.BlockRemoveObject(c, "TestBlockRemoveRetryProvisioning")
	s.assertRetryProvisioning(c, m)
}

func (s *clientSuite) TestBlockChangesRetryProvisioning(c *gc.C) {
	m := s.setupRetryProvisioning(c)
	s.BlockAllChanges(c, "TestBlockChangesRetryProvisioning")
	s.assertRetryProvisioningBlocked(c, m, "TestBlockChangesRetryProvisioning")
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
	s.PatchValue(&version.Current, current)
	result, err := s.APIState.Client().AgentVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, current)
}

func (s *clientSuite) assertDestroyMachineSuccess(c *gc.C, u *state.Unit, m0, m1, m2 *state.Machine) {
	err := s.APIState.Client().DestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the model; machine 1 has unit "wordpress/0" assigned`)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Alive)
	assertLife(c, m2, state.Dying)

	err = u.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().DestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the model`)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Dying)
	assertLife(c, m2, state.Dying)
}

func (s *clientSuite) assertBlockedErrorAndLiveliness(
	c *gc.C,
	err error,
	msg string,
	living1 state.Living,
	living2 state.Living,
	living3 state.Living,
	living4 state.Living,
) {
	s.AssertBlocked(c, err, msg)
	assertLife(c, living1, state.Alive)
	assertLife(c, living2, state.Alive)
	assertLife(c, living3, state.Alive)
	assertLife(c, living4, state.Alive)
}

func (s *clientSuite) AssertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *clientSuite) TestBlockRemoveDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)
	s.BlockRemoveObject(c, "TestBlockRemoveDestroyMachines")
	err := s.APIState.Client().DestroyMachines("0", "1", "2")
	s.assertBlockedErrorAndLiveliness(c, err, "TestBlockRemoveDestroyMachines", m0, m1, m2, u)
}

func (s *clientSuite) TestBlockChangesDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)
	s.BlockAllChanges(c, "TestBlockChangesDestroyMachines")
	err := s.APIState.Client().DestroyMachines("0", "1", "2")
	s.assertBlockedErrorAndLiveliness(c, err, "TestBlockChangesDestroyMachines", m0, m1, m2, u)
}

func (s *clientSuite) TestBlockDestoryDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)
	s.BlockDestroyModel(c, "TestBlockDestoryDestroyMachines")
	s.assertDestroyMachineSuccess(c, u, m0, m1, m2)
}

func (s *clientSuite) TestAnyBlockForceDestroyMachines(c *gc.C) {
	// force bypasses all blocks
	s.BlockAllChanges(c, "TestAnyBlockForceDestroyMachines")
	s.BlockDestroyModel(c, "TestAnyBlockForceDestroyMachines")
	s.BlockRemoveObject(c, "TestAnyBlockForceDestroyMachines")
	s.assertForceDestroyMachines(c)
}

func (s *clientSuite) assertForceDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)

	err := s.APIState.Client().ForceDestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine is required by the model`)
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

func (s *clientSuite) TestDestroyModel(c *gc.C) {
	// The full tests for DestroyModel are in modelmanager.
	// Here we just test that things are hooked up such that we can destroy
	// the model through the client endpoint to support older juju clients.
	err := s.APIState.Client().DestroyModel()
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}
