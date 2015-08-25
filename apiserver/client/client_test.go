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
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/charmrepo"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/service"
	"github.com/juju/juju/apiserver/testing"
	apiservertesting "github.com/juju/juju/apiserver/testing"
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

func (s *serverSuite) TestBlockEnsureAvailabilityDeprecated(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)

	s.BlockAllChanges(c, "TestBlockEnsureAvailabilityDeprecated")

	arg := params.StateServersSpecs{[]params.StateServersSpec{{NumStateServers: 3}}}
	results, err := s.client.EnsureAvailability(arg)
	s.AssertBlocked(c, err, "TestBlockEnsureAvailabilityDeprecated")
	c.Assert(results.Results, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
}

func (s *serverSuite) TestEnvUsersInfo(c *gc.C) {
	testAdmin := s.AdminUserTag(c)
	owner, err := s.State.EnvironmentUser(testAdmin)
	c.Assert(err, jc.ErrorIsNil)

	localUser1 := s.makeLocalEnvUser(c, "ralphdoe", "Ralph Doe")
	localUser2 := s.makeLocalEnvUser(c, "samsmith", "Sam Smith")
	remoteUser1 := s.Factory.MakeEnvUser(c, &factory.EnvUserParams{User: "bobjohns@ubuntuone", DisplayName: "Bob Johns"})
	remoteUser2 := s.Factory.MakeEnvUser(c, &factory.EnvUserParams{User: "nicshaw@idprovider", DisplayName: "Nic Shaw"})

	results, err := s.client.EnvUserInfo()
	c.Assert(err, jc.ErrorIsNil)
	var expected params.EnvUserInfoResults
	for _, r := range []struct {
		user *state.EnvironmentUser
		info *params.EnvUserInfo
	}{
		{
			owner,
			&params.EnvUserInfo{
				UserName:    owner.UserName(),
				DisplayName: owner.DisplayName(),
			},
		}, {
			localUser1,
			&params.EnvUserInfo{
				UserName:    "ralphdoe@local",
				DisplayName: "Ralph Doe",
			},
		}, {
			localUser2,
			&params.EnvUserInfo{
				UserName:    "samsmith@local",
				DisplayName: "Sam Smith",
			},
		}, {
			remoteUser1,
			&params.EnvUserInfo{
				UserName:    "bobjohns@ubuntuone",
				DisplayName: "Bob Johns",
			},
		}, {
			remoteUser2,
			&params.EnvUserInfo{
				UserName:    "nicshaw@idprovider",
				DisplayName: "Nic Shaw",
			},
		},
	} {
		r.info.CreatedBy = owner.UserName()
		r.info.DateCreated = r.user.DateCreated()
		r.info.LastConnection = lastConnPointer(c, r.user)
		expected.Results = append(expected.Results, params.EnvUserInfoResult{Result: r.info})
	}

	sort.Sort(ByUserName(expected.Results))
	sort.Sort(ByUserName(results.Results))
	c.Assert(results, jc.DeepEquals, expected)
}

func lastConnPointer(c *gc.C, envUser *state.EnvironmentUser) *time.Time {
	lastConn, err := envUser.LastConnection()
	if err != nil {
		if state.IsNeverConnectedError(err) {
			return nil
		}
		c.Fatal(err)
	}
	return &lastConn
}

// ByUserName implements sort.Interface for []params.EnvUserInfoResult based on
// the UserName field.
type ByUserName []params.EnvUserInfoResult

func (a ByUserName) Len() int           { return len(a) }
func (a ByUserName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByUserName) Less(i, j int) bool { return a[i].Result.UserName < a[j].Result.UserName }

func (s *serverSuite) makeLocalEnvUser(c *gc.C, username, displayname string) *state.EnvironmentUser {
	// factory.MakeUser will create an EnvUser for a local user by defalut
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: username, DisplayName: displayname})
	envUser, err := s.State.EnvironmentUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	return envUser
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

func (s *serverSuite) TestUnshareEnvironmentMissingUser(c *gc.C) {
	user := names.NewUserTag("bob")
	args := params.ModifyEnvironUsers{
		Changes: []params.ModifyEnvironUser{{
			UserTag: user.String(),
			Action:  params.RemoveEnvUser,
		}}}

	result, err := s.client.ShareEnvironment(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.ErrorMatches, `could not unshare environment: env user "bob@local" does not exist: transaction aborted`)

	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.NotNil)

	_, err = s.State.EnvironmentUser(user)
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
	lastConn, err := envUser.LastConnection()
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(lastConn, gc.Equals, time.Time{})
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
	lastConn, err := envUser.LastConnection()
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(lastConn.IsZero(), jc.IsTrue)
}

func (s *serverSuite) TestShareEnvironmentAddUserTwice(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar"})
	args := params.ModifyEnvironUsers{
		Changes: []params.ModifyEnvironUser{{
			UserTag: user.Tag().String(),
			Action:  params.AddEnvUser,
		}}}

	_, err := s.client.ShareEnvironment(args)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.ShareEnvironment(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.ErrorMatches, "could not share environment: environment user \"foobar@local\" already exists")
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "could not share environment: environment user \"foobar@local\" already exists")
	c.Assert(result.Results[0].Error.Code, gc.Matches, params.CodeAlreadyExists)

	envUser, err := s.State.EnvironmentUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envUser.UserName(), gc.Equals, user.UserTag().Username())
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

func (s *serverSuite) assertSetEnvironAgentVersion(c *gc.C) {
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

func (s *serverSuite) assertSetEnvironAgentVersionBlocked(c *gc.C, msg string) {
	args := params.SetEnvironAgentVersion{
		Version: version.MustParse("9.8.7"),
	}
	err := s.client.SetEnvironAgentVersion(args)
	s.AssertBlocked(c, err, msg)
}

func (s *serverSuite) TestBlockDestroySetEnvironAgentVersion(c *gc.C) {
	s.BlockDestroyEnvironment(c, "TestBlockDestroySetEnvironAgentVersion")
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
}

func (s *serverSuite) TestBlockDestroyAbortCurrentUpgrade(c *gc.C) {
	s.setupAbortCurrentUpgradeBlocked(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyAbortCurrentUpgrade")
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

func (s *serverSuite) assertServiceSetBlocked(c *gc.C, dummy *state.Service, msg string) {
	err := s.client.ServiceSet(params.ServiceSet{
		ServiceName: "dummy",
		Options: map[string]string{
			"title":    "foobar",
			"username": validSetTestValue}})
	s.AssertBlocked(c, err, msg)
}

func (s *serverSuite) assertServiceSet(c *gc.C, dummy *state.Service) {
	err := s.client.ServiceSet(params.ServiceSet{
		ServiceName: "dummy",
		Options: map[string]string{
			"title":    "foobar",
			"username": validSetTestValue}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": validSetTestValue,
	})
}

func (s *serverSuite) TestBlockDestroyServiceSet(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockDestroyEnvironment(c, "TestBlockDestroyServiceSet")
	s.assertServiceSet(c, dummy)
}

func (s *serverSuite) TestBlockRemoveServiceSet(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockRemoveObject(c, "TestBlockRemoveServiceSet")
	s.assertServiceSet(c, dummy)
}

func (s *serverSuite) TestBlockChangesServiceSet(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockAllChanges(c, "TestBlockChangesServiceSet")
	s.assertServiceSetBlocked(c, dummy, "TestBlockChangesServiceSet")
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

func (s *serverSuite) setupServerUnsetBlocked(c *gc.C) *state.Service {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	err := s.client.ServiceSet(params.ServiceSet{
		ServiceName: "dummy",
		Options: map[string]string{
			"title":    "foobar",
			"username": "user name",
		}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": "user name",
	})
	return dummy
}

func (s *serverSuite) assertServerUnset(c *gc.C, dummy *state.Service) {
	err := s.client.ServiceUnset(params.ServiceUnset{
		ServiceName: "dummy",
		Options:     []string{"username"},
	})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title": "foobar",
	})
}

func (s *serverSuite) assertServerUnsetBlocked(c *gc.C, dummy *state.Service, msg string) {
	err := s.client.ServiceUnset(params.ServiceUnset{
		ServiceName: "dummy",
		Options:     []string{"username"},
	})
	s.AssertBlocked(c, err, msg)
}

func (s *serverSuite) TestBlockDestroyServerUnset(c *gc.C) {
	dummy := s.setupServerUnsetBlocked(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyServerUnset")
	s.assertServerUnset(c, dummy)
}

func (s *serverSuite) TestBlockRemoveServerUnset(c *gc.C) {
	dummy := s.setupServerUnsetBlocked(c)
	s.BlockRemoveObject(c, "TestBlockRemoveServerUnset")
	s.assertServerUnset(c, dummy)
}

func (s *serverSuite) TestBlockChangesServerUnset(c *gc.C) {
	dummy := s.setupServerUnsetBlocked(c)
	s.BlockAllChanges(c, "TestBlockChangesServerUnset")
	s.assertServerUnsetBlocked(c, dummy, "TestBlockChangesServerUnset")
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

func (s *clientSuite) assertServiceSetYAML(c *gc.C, dummy *state.Service) {
	err := s.APIState.Client().ServiceSetYAML("dummy", "dummy:\n  title: foobar\n  username: user name\n")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": "user name",
	})
}

func (s *clientSuite) assertServiceSetYAMLBlocked(c *gc.C, dummy *state.Service, msg string) {
	err := s.APIState.Client().ServiceSetYAML("dummy", "dummy:\n  title: foobar\n  username: user name\n")
	s.AssertBlocked(c, err, msg)
}

func (s *clientSuite) TestBlockDestroyServiceSetYAML(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockDestroyEnvironment(c, "TestBlockDestroyServiceSetYAML")
	s.assertServiceSetYAML(c, dummy)
}

func (s *clientSuite) TestBlockRemoveServiceSetYAML(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockRemoveObject(c, "TestBlockRemoveServiceSetYAML")
	s.assertServiceSetYAML(c, dummy)
}

func (s *clientSuite) TestBlockChangesServiceSetYAML(c *gc.C) {
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockAllChanges(c, "TestBlockChangesServiceSetYAML")
	s.assertServiceSetYAMLBlocked(c, dummy, "TestBlockChangesServiceSetYAML")
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

func (s *clientSuite) TestClientAddServiceUnitsToNewContainer(c *gc.C) {
	svc := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.APIState.Client().AddServiceUnits("dummy", 1, "lxc:"+machine.Id())
	c.Assert(err, jc.ErrorIsNil)

	units, err := svc.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id()+"/lxc/0")
}

var clientAddServiceUnitsWithPlacementTests = []struct {
	about      string
	service    string // if not set, defaults to 'dummy'
	expected   []string
	machineIds []string
	placement  []*instance.Placement
	err        string
}{
	{
		about:      "valid placement directives",
		expected:   []string{"dummy/0"},
		placement:  []*instance.Placement{{"deadbeef-0bad-400d-8000-4b1d0d06f00d", "valid"}},
		machineIds: []string{"1"},
	}, {
		about:      "direct machine assignment placement directive",
		expected:   []string{"dummy/1", "dummy/2"},
		placement:  []*instance.Placement{{"#", "1"}, {"lxc", "1"}},
		machineIds: []string{"1", "1/lxc/0"},
	}, {
		about:     "invalid placement directive",
		err:       ".* invalid placement is invalid",
		expected:  []string{"dummy/3"},
		placement: []*instance.Placement{{"deadbeef-0bad-400d-8000-4b1d0d06f00d", "invalid"}},
	},
}

func (s *clientSuite) TestClientAddServiceUnitsWithPlacement(c *gc.C) {
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	// Add a machine for the units to be placed on.
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	for i, t := range clientAddServiceUnitsWithPlacementTests {
		c.Logf("test %d. %s", i, t.about)
		serviceName := t.service
		if serviceName == "" {
			serviceName = "dummy"
		}
		units, err := s.APIState.Client().AddServiceUnitsWithPlacement(serviceName, len(t.expected), t.placement)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(units, gc.DeepEquals, t.expected)
		for i, unitName := range units {
			u, err := s.BackingState.Unit(unitName)
			c.Assert(err, jc.ErrorIsNil)
			assignedMachine, err := u.AssignedMachineId()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(assignedMachine, gc.Equals, t.machineIds[i])
		}
	}
}

func (s *clientSuite) assertAddServiceUnits(c *gc.C) {
	units, err := s.APIState.Client().AddServiceUnits("dummy", 3, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.DeepEquals, []string{"dummy/0", "dummy/1", "dummy/2"})

	// Test that we actually assigned the unit to machine 0
	forcedUnit, err := s.BackingState.Unit("dummy/0")
	c.Assert(err, jc.ErrorIsNil)
	assignedMachine, err := forcedUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(assignedMachine, gc.Equals, "0")
}

func (s *clientSuite) assertAddServiceUnitsBlocked(c *gc.C, msg string) {
	_, err := s.APIState.Client().AddServiceUnits("dummy", 3, "")
	s.AssertBlocked(c, err, msg)
}

func (s *clientSuite) TestBlockDestroyAddServiceUnits(c *gc.C) {
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockDestroyEnvironment(c, "TestBlockDestroyAddServiceUnits")
	s.assertAddServiceUnits(c)
}

func (s *clientSuite) TestBlockRemoveAddServiceUnits(c *gc.C) {
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockRemoveObject(c, "TestBlockRemoveAddServiceUnits")
	s.assertAddServiceUnits(c)
}

func (s *clientSuite) TestBlockChangeAddServiceUnits(c *gc.C) {
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockAllChanges(c, "TestBlockChangeAddServiceUnits")
	s.assertAddServiceUnitsBlocked(c, "TestBlockChangeAddServiceUnits")
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
	c.Assert(info.ServerUUID, gc.Equals, env.ServerUUID())
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
		state.Entity
	}
	entities := []taggedAnnotator{service, unit, machine, environment}
	for i, t := range clientAnnotationsTests {
		for _, entity := range entities {
			id := entity.Tag().String() // this is WRONG, it should be Tag().Id() but the code is wrong.
			c.Logf("test %d. %s. entity %s", i, t.about, id)
			// Set initial entity annotations.
			err := s.APIState.Client().SetAnnotations(id, t.initial)
			c.Assert(err, jc.ErrorIsNil)
			// Add annotations using the API call.
			err = s.APIState.Client().SetAnnotations(id, t.input)
			if t.err != "" {
				c.Assert(err, gc.ErrorMatches, t.err)
				continue
			}
			// Retrieve annotations using the API call.
			ann, err := s.APIState.Client().GetAnnotations(id)
			c.Assert(err, jc.ErrorIsNil)
			// Check annotations are correctly returned.
			c.Assert(ann, gc.DeepEquals, t.input)
			// Clean up annotations on the current entity.
			cleanup := make(map[string]string)
			for key := range ann {
				cleanup[key] = ""
			}
			err = s.APIState.Client().SetAnnotations(id, cleanup)
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

func (s *clientSuite) TestCharmAnnotationsUnsupported(c *gc.C) {
	// Set up charm.
	charm := s.AddTestingCharm(c, "dummy")
	id := charm.Tag().Id()
	for i, t := range clientAnnotationsTests {
		c.Logf("test %d. %s. entity %s", i, t.about, id)
		// Add annotations using the API call.
		err := s.APIState.Client().SetAnnotations(id, t.input)
		// Should not be able to annotate charm with this client
		c.Assert(err.Error(), gc.Matches, ".*is not a valid tag.*")

		// Retrieve annotations using the API call.
		ann, err := s.APIState.Client().GetAnnotations(id)
		// Should not be able to get annotations from charm using this client
		c.Assert(err.Error(), gc.Matches, ".*is not a valid tag.*")
		c.Assert(ann, gc.IsNil)
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

func (s *clientSuite) setupServiceExpose(c *gc.C) {
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
}

func (s *clientSuite) assertServiceExpose(c *gc.C) {
	for i, t := range serviceExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.APIState.Client().ServiceExpose(t.service)
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

func (s *clientSuite) assertServiceExposeBlocked(c *gc.C, msg string) {
	for i, t := range serviceExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.APIState.Client().ServiceExpose(t.service)
		s.AssertBlocked(c, err, msg)
	}
}

func (s *clientSuite) TestBlockDestroyServiceExpose(c *gc.C) {
	s.setupServiceExpose(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyServiceExpose")
	s.assertServiceExpose(c)
}

func (s *clientSuite) TestBlockRemoveServiceExpose(c *gc.C) {
	s.setupServiceExpose(c)
	s.BlockRemoveObject(c, "TestBlockRemoveServiceExpose")
	s.assertServiceExpose(c)
}

func (s *clientSuite) TestBlockChangesServiceExpose(c *gc.C) {
	s.setupServiceExpose(c)
	s.BlockAllChanges(c, "TestBlockChangesServiceExpose")
	s.assertServiceExposeBlocked(c, "TestBlockChangesServiceExpose")
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

func (s *clientSuite) setupServiceUnexpose(c *gc.C) *state.Service {
	charm := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "dummy-service", charm)
	svc.SetExposed()
	c.Assert(svc.IsExposed(), gc.Equals, true)
	return svc
}

func (s *clientSuite) assertServiceUnexpose(c *gc.C, svc *state.Service) {
	err := s.APIState.Client().ServiceUnexpose("dummy-service")
	c.Assert(err, jc.ErrorIsNil)
	svc.Refresh()
	c.Assert(svc.IsExposed(), gc.Equals, false)
	err = svc.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) assertServiceUnexposeBlocked(c *gc.C, svc *state.Service, msg string) {
	err := s.APIState.Client().ServiceUnexpose("dummy-service")
	s.AssertBlocked(c, err, msg)
	err = svc.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestBlockDestroyServiceUnexpose(c *gc.C) {
	svc := s.setupServiceUnexpose(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyServiceUnexpose")
	s.assertServiceUnexpose(c, svc)
}

func (s *clientSuite) TestBlockRemoveServiceUnexpose(c *gc.C) {
	svc := s.setupServiceUnexpose(c)
	s.BlockRemoveObject(c, "TestBlockRemoveServiceUnexpose")
	s.assertServiceUnexpose(c, svc)
}

func (s *clientSuite) TestBlockChangesServiceUnexpose(c *gc.C) {
	svc := s.setupServiceUnexpose(c)
	s.BlockAllChanges(c, "TestBlockChangesServiceUnexpose")
	s.assertServiceUnexposeBlocked(c, svc, "TestBlockChangesServiceUnexpose")
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
		err = unit.SetAgentStatus(state.StatusIdle, "", nil)
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
	s.BlockDestroyEnvironment(c, "TestBlockDestroyUnitResolved")
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
	apiservertesting.CharmStoreSuite
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
}

func (s *clientRepoSuite) TearDownTest(c *gc.C) {
	s.CharmStoreSuite.TearDownTest(c)
	s.baseSuite.TearDownTest(c)
}

func (s *clientRepoSuite) TestClientServiceDeployCharmErrors(c *gc.C) {
	for url, expect := range map[string]string{
		"wordpress":                   "charm url series is not resolved",
		"cs:wordpress":                "charm url series is not resolved",
		"cs:precise/wordpress":        "charm url must include revision",
		"cs:precise/wordpress-999999": `.* charm "cs:precise/wordpress-999999".* not found`,
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

func (s *clientRepoSuite) TestClientServiceDeployWithNetworks(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("mem=4G networks=^net3")

	// Check for invalid network tags handling.
	err = s.APIState.Client().ServiceDeployWithNetworks(
		curl.String(), "service", 3, "", cons, "",
		[]string{"net1", "net2"},
	)
	c.Assert(err, gc.ErrorMatches, `"net1" is not a valid tag`)

	err = s.APIState.Client().ServiceDeployWithNetworks(
		curl.String(), "service", 3, "", cons, "",
		[]string{"network-net1", "network-net2"},
	)
	c.Assert(err, gc.ErrorMatches, "use of --networks is deprecated. Please use spaces")
}

func (s *clientRepoSuite) setupServiceDeploy(c *gc.C, args string) (*charm.URL, charm.Charm, constraints.Value) {
	curl, ch := s.UploadCharm(c, "precise/dummy-42", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse(args)
	return curl, ch, cons
}

func (s *clientRepoSuite) TestClientServiceDeployPrincipal(c *gc.C) {
	// TODO(fwereade): test ToMachineSpec directly on srvClient, when we
	// manage to extract it as a package and can thus do it conveniently.
	curl, ch := s.UploadCharm(c, "trusty/dummy-1", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	mem4g := constraints.MustParse("mem=4G")
	err = s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", mem4g, "",
	)
	c.Assert(err, jc.ErrorIsNil)
	apiservertesting.AssertPrincipalServiceDeployed(c, s.State, "service", curl, false, ch, mem4g)
}

func (s *clientRepoSuite) assertServiceDeployPrincipal(c *gc.C, curl *charm.URL, ch charm.Charm, mem4g constraints.Value) {
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", mem4g, "",
	)
	c.Assert(err, jc.ErrorIsNil)
	apiservertesting.AssertPrincipalServiceDeployed(c, s.State, "service", curl, false, ch, mem4g)
}

func (s *clientRepoSuite) assertServiceDeployPrincipalBlocked(c *gc.C, msg string, curl *charm.URL, mem4g constraints.Value) {
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", mem4g, "",
	)
	s.AssertBlocked(c, err, msg)
}

func (s *clientRepoSuite) TestBlockDestroyServiceDeployPrincipal(c *gc.C) {
	curl, bundle, cons := s.setupServiceDeploy(c, "mem=4G")
	s.BlockDestroyEnvironment(c, "TestBlockDestroyServiceDeployPrincipal")
	s.assertServiceDeployPrincipal(c, curl, bundle, cons)
}

func (s *clientRepoSuite) TestBlockRemoveServiceDeployPrincipal(c *gc.C) {
	curl, bundle, cons := s.setupServiceDeploy(c, "mem=4G")
	s.BlockRemoveObject(c, "TestBlockRemoveServiceDeployPrincipal")
	s.assertServiceDeployPrincipal(c, curl, bundle, cons)
}

func (s *clientRepoSuite) TestBlockChangesServiceDeployPrincipal(c *gc.C) {
	curl, _, cons := s.setupServiceDeploy(c, "mem=4G")
	s.BlockAllChanges(c, "TestBlockChangesServiceDeployPrincipal")
	s.assertServiceDeployPrincipalBlocked(c, "TestBlockChangesServiceDeployPrincipal", curl, cons)
}

func (s *clientRepoSuite) TestClientServiceDeploySubordinate(c *gc.C) {
	curl, ch := s.UploadCharm(c, "utopic/logging-47", "logging")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 0, "", constraints.Value{}, "",
	)
	service, err := s.State.Service("service-name")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	c.Assert(charm.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, ch.Config())

	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)
}

func (s *clientRepoSuite) TestClientServiceDeployConfig(c *gc.C) {
	// TODO(fwereade): test Config/ConfigYAML handling directly on srvClient.
	// Can't be done cleanly until it's extracted similarly to Machiner.
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 1, "service-name:\n  username: fred", constraints.Value{}, "",
	)
	c.Assert(err, jc.ErrorIsNil)
	service, err := s.State.Service("service-name")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"username": "fred"})
}

func (s *clientRepoSuite) TestClientServiceDeployConfigError(c *gc.C) {
	// TODO(fwereade): test Config/ConfigYAML handling directly on srvClient.
	// Can't be done cleanly until it's extracted similarly to Machiner.
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 1, "service-name:\n  skill-level: fred", constraints.Value{}, "",
	)
	c.Assert(err, gc.ErrorMatches, `option "skill-level" expected int, got "fred"`)
	_, err = s.State.Service("service-name")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *clientRepoSuite) TestClientServiceDeployToMachine(c *gc.C) {
	curl, ch := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(charm.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, ch.Config())

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

func (s *clientRepoSuite) TestClientServiceDeployServiceOwner(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)

	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "password"})
	s.APIState = s.OpenAPIAs(c, user.Tag(), "password")

	err = s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{}, "",
	)
	c.Assert(err, jc.ErrorIsNil)

	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(service.GetOwnerTag(), gc.Equals, user.Tag().String())
}

func (s *clientRepoSuite) deployServiceForTests(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-1", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().ServiceDeploy(curl.String(),
		"service", 1, "", constraints.Value{}, "",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientRepoSuite) checkClientServiceUpdateSetCharm(c *gc.C, forceCharmUrl bool) {
	s.deployServiceForTests(c)
	s.UploadCharm(c, "precise/wordpress-3", "wordpress")

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

func (s *clientRepoSuite) TestClientServiceUpdateSetCharm(c *gc.C) {
	s.checkClientServiceUpdateSetCharm(c, false)
}

func (s *clientRepoSuite) TestBlockDestroyServiceUpdate(c *gc.C) {
	s.BlockDestroyEnvironment(c, "TestBlockDestroyServiceUpdate")
	s.checkClientServiceUpdateSetCharm(c, false)
}

func (s *clientRepoSuite) TestBlockRemoveServiceUpdate(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveServiceUpdate")
	s.checkClientServiceUpdateSetCharm(c, false)
}

func (s *clientRepoSuite) setupServiceUpdate(c *gc.C) {
	s.deployServiceForTests(c)
	s.UploadCharm(c, "precise/wordpress-3", "wordpress")
}

func (s *clientRepoSuite) TestBlockChangeServiceUpdate(c *gc.C) {
	s.setupServiceUpdate(c)
	s.BlockAllChanges(c, "TestBlockChangeServiceUpdate")
	// Update the charm for the service.
	args := params.ServiceUpdate{
		ServiceName:   "service",
		CharmUrl:      "cs:precise/wordpress-3",
		ForceCharmUrl: false,
	}
	err := s.APIState.Client().ServiceUpdate(args)
	s.AssertBlocked(c, err, "TestBlockChangeServiceUpdate")
}

func (s *clientRepoSuite) TestClientServiceUpdateForceSetCharm(c *gc.C) {
	s.checkClientServiceUpdateSetCharm(c, true)
}

func (s *clientRepoSuite) TestBlockServiceUpdateForced(c *gc.C) {
	s.setupServiceUpdate(c)

	// block all changes. Force should ignore block :)
	s.BlockAllChanges(c, "TestBlockServiceUpdateForced")
	s.BlockDestroyEnvironment(c, "TestBlockServiceUpdateForced")
	s.BlockRemoveObject(c, "TestBlockServiceUpdateForced")

	// Update the charm for the service.
	args := params.ServiceUpdate{
		ServiceName:   "service",
		CharmUrl:      "cs:precise/wordpress-3",
		ForceCharmUrl: true,
	}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the charm has been updated and and the force flag correctly set.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, "cs:precise/wordpress-3")
	c.Assert(force, jc.IsTrue)
}

func (s *clientRepoSuite) TestClientServiceUpdateSetCharmErrors(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	for charmUrl, expect := range map[string]string{
		"wordpress":                   "charm url series is not resolved",
		"cs:wordpress":                "charm url series is not resolved",
		"cs:precise/wordpress":        "charm url must include revision",
		"cs:precise/wordpress-999999": `cannot retrieve charm "cs:precise/wordpress-999999": charm not found`,
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

func (s *clientRepoSuite) TestClientServiceUpdateAllParams(c *gc.C) {
	s.deployServiceForTests(c)
	s.UploadCharm(c, "precise/wordpress-3", "wordpress")

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

func (s *clientRepoSuite) TestClientServiceSetCharm(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{}, "",
	)
	c.Assert(err, jc.ErrorIsNil)
	s.UploadCharm(c, "precise/wordpress-3", "wordpress")
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

func (s *clientRepoSuite) setupServiceSetCharm(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{}, "",
	)
	c.Assert(err, jc.ErrorIsNil)
	s.UploadCharm(c, "precise/wordpress-3", "wordpress")
}

func (s *clientRepoSuite) assertServiceSetCharm(c *gc.C, force bool) {
	err := s.APIState.Client().ServiceSetCharm(
		"service", "cs:precise/wordpress-3", force,
	)
	c.Assert(err, jc.ErrorIsNil)
	// Ensure that the charm is not marked as forced.
	service, err := s.State.Service("service")
	c.Assert(err, jc.ErrorIsNil)
	charm, _, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charm.URL().String(), gc.Equals, "cs:precise/wordpress-3")
}

func (s *clientRepoSuite) assertServiceSetCharmBlocked(c *gc.C, force bool, msg string) {
	err := s.APIState.Client().ServiceSetCharm(
		"service", "cs:precise/wordpress-3", force,
	)
	s.AssertBlocked(c, err, msg)
}

func (s *clientRepoSuite) TestBlockDestroyServiceSetCharm(c *gc.C) {
	s.setupServiceSetCharm(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyServiceSetCharm")
	s.assertServiceSetCharm(c, false)
}

func (s *clientRepoSuite) TestBlockRemoveServiceSetCharm(c *gc.C) {
	s.setupServiceSetCharm(c)
	s.BlockRemoveObject(c, "TestBlockRemoveServiceSetCharm")
	s.assertServiceSetCharm(c, false)
}

func (s *clientRepoSuite) TestBlockChangesServiceSetCharm(c *gc.C) {
	s.setupServiceSetCharm(c)
	s.BlockAllChanges(c, "TestBlockChangesServiceSetCharm")
	s.assertServiceSetCharmBlocked(c, false, "TestBlockChangesServiceSetCharm")
}

func (s *clientRepoSuite) TestClientServiceSetCharmForce(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{}, "",
	)
	c.Assert(err, jc.ErrorIsNil)
	s.UploadCharm(c, "precise/wordpress-3", "wordpress")
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

func (s *clientRepoSuite) TestBlockServiceSetCharmForce(c *gc.C) {
	s.setupServiceSetCharm(c)

	// block all changes
	s.BlockAllChanges(c, "TestBlockServiceSetCharmForce")
	s.BlockRemoveObject(c, "TestBlockServiceSetCharmForce")
	s.BlockDestroyEnvironment(c, "TestBlockServiceSetCharmForce")

	s.assertServiceSetCharm(c, true)
}

func (s *clientSuite) TestClientServiceSetCharmInvalidService(c *gc.C) {
	err := s.APIState.Client().ServiceSetCharm(
		"badservice", "cs:precise/wordpress-3", true,
	)
	c.Assert(err, gc.ErrorMatches, `service "badservice" not found`)
}

func (s *clientRepoSuite) TestClientServiceSetCharmErrors(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	for url, expect := range map[string]string{
		// TODO(fwereade,Makyo) make these errors consistent one day.
		"wordpress":                   "charm url series is not resolved",
		"cs:wordpress":                "charm url series is not resolved",
		"cs:precise/wordpress":        "charm url must include revision",
		"cs:precise/wordpress-999999": `cannot retrieve charm "cs:precise/wordpress-999999": charm not found`,
	} {
		c.Logf("test %s", url)
		err := s.APIState.Client().ServiceSetCharm(
			"wordpress", url, false,
		)
		c.Check(err, gc.ErrorMatches, expect)
	}
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(rels), gc.Equals, 1)
}

func (s *clientSuite) TestSuccessfullyAddRelation(c *gc.C) {
	endpoints := []string{"wordpress", "mysql"}
	s.assertAddRelation(c, endpoints)
}

func (s *clientSuite) TestBlockDestroyAddRelation(c *gc.C) {
	s.BlockDestroyEnvironment(c, "TestBlockDestroyAddRelation")
	s.assertAddRelation(c, []string{"wordpress", "mysql"})
}
func (s *clientSuite) TestBlockRemoveAddRelation(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveAddRelation")
	s.assertAddRelation(c, []string{"wordpress", "mysql"})
}

func (s *clientSuite) TestBlockChangesAddRelation(c *gc.C) {
	s.setUpScenario(c)
	s.BlockAllChanges(c, "TestBlockChangesAddRelation")
	_, err := s.APIState.Client().AddRelation([]string{"wordpress", "mysql"}...)
	s.AssertBlocked(c, err, "TestBlockChangesAddRelation")
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
			EnvUUID:                 s.State.EnvironUUID(),
			Id:                      m.Id(),
			InstanceId:              "i-0",
			Status:                  multiwatcher.Status("pending"),
			StatusData:              map[string]interface{}{},
			Life:                    multiwatcher.Life("alive"),
			Series:                  "quantal",
			Jobs:                    []multiwatcher.MachineJob{state.JobManageEnviron.ToParams()},
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

func (s *clientSuite) setupSetServiceConstraints(c *gc.C) (*state.Service, constraints.Value) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
	// Update constraints for the service.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	return service, cons
}

func (s *clientSuite) assertSetServiceConstraints(c *gc.C, service *state.Service, cons constraints.Value) {
	err := s.APIState.Client().SetServiceConstraints("dummy", cons)
	c.Assert(err, jc.ErrorIsNil)
	// Ensure the constraints have been correctly updated.
	obtained, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) assertSetServiceConstraintsBlocked(c *gc.C, msg string, service *state.Service, cons constraints.Value) {
	err := s.APIState.Client().SetServiceConstraints("dummy", cons)
	s.AssertBlocked(c, err, msg)
}

func (s *clientSuite) TestBlockDestroySetServiceConstraints(c *gc.C) {
	svc, cons := s.setupSetServiceConstraints(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroySetServiceConstraints")
	s.assertSetServiceConstraints(c, svc, cons)
}

func (s *clientSuite) TestBlockRemoveSetServiceConstraints(c *gc.C) {
	svc, cons := s.setupSetServiceConstraints(c)
	s.BlockRemoveObject(c, "TestBlockRemoveSetServiceConstraints")
	s.assertSetServiceConstraints(c, svc, cons)
}

func (s *clientSuite) TestBlockChangesSetServiceConstraints(c *gc.C) {
	svc, cons := s.setupSetServiceConstraints(c)
	s.BlockAllChanges(c, "TestBlockChangesSetServiceConstraints")
	s.assertSetServiceConstraintsBlocked(c, "TestBlockChangesSetServiceConstraints", svc, cons)
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

func (s *clientSuite) assertSetEnvironmentConstraints(c *gc.C) {
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

func (s *clientSuite) assertSetEnvironmentConstraintsBlocked(c *gc.C, msg string) {
	// Set constraints for the environment.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().SetEnvironmentConstraints(cons)
	s.AssertBlocked(c, err, msg)
}

func (s *clientSuite) TestBlockDestroyClientSetEnvironmentConstraints(c *gc.C) {
	s.BlockDestroyEnvironment(c, "TestBlockDestroyClientSetEnvironmentConstraints")
	s.assertSetEnvironmentConstraints(c)
}

func (s *clientSuite) TestBlockRemoveClientSetEnvironmentConstraints(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveClientSetEnvironmentConstraints")
	s.assertSetEnvironmentConstraints(c)
}

func (s *clientSuite) TestBlockChangesClientSetEnvironmentConstraints(c *gc.C) {
	s.BlockAllChanges(c, "TestBlockChangesClientSetEnvironmentConstraints")
	s.assertSetEnvironmentConstraintsBlocked(c, "TestBlockChangesClientSetEnvironmentConstraints")
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

func (s *serverSuite) TestClientEnvironmentGet(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.client.EnvironmentGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config, gc.DeepEquals, envConfig.AllAttrs())
}

func (s *serverSuite) assertEnvValue(c *gc.C, key string, expected interface{}) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	value, found := envConfig.AllAttrs()[key]
	c.Assert(found, jc.IsTrue)
	c.Assert(value, gc.Equals, expected)
}

func (s *serverSuite) assertEnvValueMissing(c *gc.C, key string) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found := envConfig.AllAttrs()[key]
	c.Assert(found, jc.IsFalse)
}

func (s *serverSuite) TestClientEnvironmentSet(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found := envConfig.AllAttrs()["some-key"]
	c.Assert(found, jc.IsFalse)

	params := params.EnvironmentSet{
		Config: map[string]interface{}{
			"some-key":  "value",
			"other-key": "other value"},
	}
	err = s.client.EnvironmentSet(params)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvValue(c, "some-key", "value")
	s.assertEnvValue(c, "other-key", "other value")
}

func (s *serverSuite) TestClientEnvironmentSetImmutable(c *gc.C) {
	// The various immutable config values are tested in
	// environs/config/config_test.go, so just choosing one here.
	params := params.EnvironmentSet{
		Config: map[string]interface{}{"state-port": "1"},
	}
	err := s.client.EnvironmentSet(params)
	c.Check(err, gc.ErrorMatches, `cannot change state-port from .* to 1`)
}

func (s *serverSuite) assertEnvironmentSetBlocked(c *gc.C, args map[string]interface{}, msg string) {
	err := s.client.EnvironmentSet(params.EnvironmentSet{args})
	s.AssertBlocked(c, err, msg)
}

func (s *serverSuite) TestBlockChangesClientEnvironmentSet(c *gc.C) {
	s.BlockAllChanges(c, "TestBlockChangesClientEnvironmentSet")
	args := map[string]interface{}{"some-key": "value"}
	s.assertEnvironmentSetBlocked(c, args, "TestBlockChangesClientEnvironmentSet")
}

func (s *serverSuite) TestClientEnvironmentSetDeprecated(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	url := envConfig.AllAttrs()["agent-metadata-url"]
	c.Assert(url, gc.Equals, "")

	args := params.EnvironmentSet{
		Config: map[string]interface{}{"tools-metadata-url": "value"},
	}
	err = s.client.EnvironmentSet(args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvValue(c, "agent-metadata-url", "value")
	s.assertEnvValue(c, "tools-metadata-url", "value")
}

func (s *serverSuite) TestClientEnvironmentSetCannotChangeAgentVersion(c *gc.C) {
	args := params.EnvironmentSet{
		map[string]interface{}{"agent-version": "9.9.9"},
	}
	err := s.client.EnvironmentSet(args)
	c.Assert(err, gc.ErrorMatches, "agent-version cannot be changed")

	// It's okay to pass env back with the same agent-version.
	result, err := s.client.EnvironmentGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config["agent-version"], gc.NotNil)
	args.Config["agent-version"] = result.Config["agent-version"]
	err = s.client.EnvironmentSet(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) TestClientEnvironmentUnset(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"abc": 123}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	args := params.EnvironmentUnset{[]string{"abc"}}
	err = s.client.EnvironmentUnset(args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvValueMissing(c, "abc")
}

func (s *serverSuite) TestBlockClientEnvironmentUnset(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"abc": 123}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.BlockAllChanges(c, "TestBlockClientEnvironmentUnset")

	args := params.EnvironmentUnset{[]string{"abc"}}
	err = s.client.EnvironmentUnset(args)
	s.AssertBlocked(c, err, "TestBlockClientEnvironmentUnset")
}

func (s *serverSuite) TestClientEnvironmentUnsetMissing(c *gc.C) {
	// It's okay to unset a non-existent attribute.
	args := params.EnvironmentUnset{[]string{"not_there"}}
	err := s.client.EnvironmentUnset(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) TestClientEnvironmentUnsetError(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"abc": 123}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// "type" may not be removed, and this will cause an error.
	// If any one attribute's removal causes an error, there
	// should be no change.
	args := params.EnvironmentUnset{[]string{"abc", "type"}}
	err = s.client.EnvironmentUnset(args)
	c.Assert(err, gc.ErrorMatches, "type: expected string, got nothing")
	s.assertEnvValue(c, "abc", 123)
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
	url := fmt.Sprintf("https://%s/environment/%s/tools/%s",
		s.APIState.Addr(), coretesting.EnvironmentTag.Id(), result.List[0].Version)
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
	s.BlockDestroyEnvironment(c, "TestBlockDestroyClientAddMachinesDefaultSeries")
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
	err := s.State.UpdateEnvironConfig(map[string]interface{}{key: block}, nil, nil)
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

type testModeCharmRepo struct {
	*charmrepo.CharmStore
	testMode bool
}

// WithTestMode returns a repository Interface where test mode is enabled.
func (s *testModeCharmRepo) WithTestMode() charmrepo.Interface {
	s.testMode = true
	return s.CharmStore.WithTestMode()
}

func (s *clientRepoSuite) TestClientSpecializeStoreOnDeployServiceSetCharmAndAddCharm(c *gc.C) {
	repo := &testModeCharmRepo{}
	s.PatchValue(&service.NewCharmStore, func(p charmrepo.NewCharmStoreParams) charmrepo.Interface {
		p.URL = s.Srv.URL()
		repo.CharmStore = charmrepo.NewCharmStore(p).(*charmrepo.CharmStore)
		return repo
	})
	attrs := map[string]interface{}{"test-mode": true}
	err := s.State.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the store's test mode is enabled when calling ServiceDeploy.
	curl, _ := s.UploadCharm(c, "trusty/dummy-1", "dummy")
	err = service.AddCharmWithAuthorization(s.State, params.AddCharmWithAuthorization{URL: curl.String()})
	c.Assert(err, jc.ErrorIsNil)
	err = s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{}, "",
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(repo.testMode, jc.IsTrue)

	// Check that the store's test mode is enabled when calling ServiceSetCharm.
	curl, _ = s.UploadCharm(c, "trusty/wordpress-2", "wordpress")
	err = s.APIState.Client().ServiceSetCharm(
		"service", curl.String(), false,
	)
	c.Assert(repo.testMode, jc.IsTrue)

	// Check that the store's test mode is enabled when calling AddCharm.
	curl, _ = s.UploadCharm(c, "utopic/riak-42", "riak")
	err = s.APIState.Client().AddCharm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(repo.testMode, jc.IsTrue)
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
	resolveErr: `cannot resolve charm URL "cs:utopic/riak-42": charm not found`,
}, {
	about:      "reference not found",
	url:        "cs:no-such",
	resolveErr: `cannot resolve charm URL "cs:no-such": charm not found`,
}, {
	about:    "invalid charm name",
	url:      "cs:",
	parseErr: `charm URL has invalid charm name: "cs:"`,
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
		ref, err := charm.ParseReference(test.url)
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
	s.BlockDestroyEnvironment(c, "TestBlockDestroyRetryProvisioning")
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
	s.PatchValue(&version.Current.Number, current)
	result, err := s.APIState.Client().AgentVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, current)
}

func (s *serverSuite) TestBlockServiceDestroy(c *gc.C) {
	s.AddTestingService(c, "dummy-service", s.AddTestingCharm(c, "dummy"))

	// block remove-objects
	s.BlockRemoveObject(c, "TestBlockServiceDestroy")
	err := s.APIState.Client().ServiceDestroy("dummy-service")
	s.AssertBlocked(c, err, "TestBlockServiceDestroy")
	// Tests may have invalid service names.
	service, err := s.State.Service("dummy-service")
	if err == nil {
		// For valid service names, check that service is alive :-)
		assertLife(c, service, state.Alive)
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
	s.BlockDestroyEnvironment(c, "TestBlockDestoryDestroyMachines")
	s.assertDestroyMachineSuccess(c, u, m0, m1, m2)
}

func (s *clientSuite) TestAnyBlockForceDestroyMachines(c *gc.C) {
	// force bypasses all blocks
	s.BlockAllChanges(c, "TestAnyBlockForceDestroyMachines")
	s.BlockDestroyEnvironment(c, "TestAnyBlockForceDestroyMachines")
	s.BlockRemoveObject(c, "TestAnyBlockForceDestroyMachines")
	s.assertForceDestroyMachines(c)
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

func (s *clientSuite) setupDestroyPrincipalUnits(c *gc.C) []*state.Unit {
	units := make([]*state.Unit, 5)
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	for i := range units {
		unit, err := wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = unit.SetAgentStatus(state.StatusIdle, "", nil)
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
	}
	return units
}
func (s *clientSuite) TestBlockChangesDestroyPrincipalUnits(c *gc.C) {
	units := s.setupDestroyPrincipalUnits(c)
	s.BlockAllChanges(c, "TestBlockChangesDestroyPrincipalUnits")
	err := s.APIState.Client().DestroyServiceUnits("wordpress/0", "wordpress/1")
	s.assertBlockedErrorAndLiveliness(c, err, "TestBlockChangesDestroyPrincipalUnits", units[0], units[1], units[2], units[3])
}

func (s *clientSuite) TestBlockRemoveDestroyPrincipalUnits(c *gc.C) {
	units := s.setupDestroyPrincipalUnits(c)
	s.BlockRemoveObject(c, "TestBlockRemoveDestroyPrincipalUnits")
	err := s.APIState.Client().DestroyServiceUnits("wordpress/0", "wordpress/1")
	s.assertBlockedErrorAndLiveliness(c, err, "TestBlockRemoveDestroyPrincipalUnits", units[0], units[1], units[2], units[3])
}

func (s *clientSuite) TestBlockDestroyDestroyPrincipalUnits(c *gc.C) {
	units := s.setupDestroyPrincipalUnits(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyDestroyPrincipalUnits")
	err := s.APIState.Client().DestroyServiceUnits("wordpress/0", "wordpress/1")
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, units[0], state.Dying)
	assertLife(c, units[1], state.Dying)
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

func (s *clientSuite) TestBlockRemoveDestroySubordinateUnits(c *gc.C) {
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

	s.BlockRemoveObject(c, "TestBlockRemoveDestroySubordinateUnits")
	// Try to destroy the subordinate alone; check it fails.
	err = s.APIState.Client().DestroyServiceUnits("logging/0")
	s.AssertBlocked(c, err, "TestBlockRemoveDestroySubordinateUnits")
	assertLife(c, rel, state.Alive)
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)

	err = s.APIState.Client().DestroyServiceUnits("wordpress/0", "logging/0")
	s.AssertBlocked(c, err, "TestBlockRemoveDestroySubordinateUnits")
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)
	assertLife(c, rel, state.Alive)
}

func (s *clientSuite) TestBlockChangesDestroySubordinateUnits(c *gc.C) {
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

	s.BlockAllChanges(c, "TestBlockChangesDestroySubordinateUnits")
	// Try to destroy the subordinate alone; check it fails.
	err = s.APIState.Client().DestroyServiceUnits("logging/0")
	s.AssertBlocked(c, err, "TestBlockChangesDestroySubordinateUnits")
	assertLife(c, rel, state.Alive)
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)

	err = s.APIState.Client().DestroyServiceUnits("wordpress/0", "logging/0")
	s.AssertBlocked(c, err, "TestBlockChangesDestroySubordinateUnits")
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)
	assertLife(c, rel, state.Alive)
}

func (s *clientSuite) TestBlockDestroyDestroySubordinateUnits(c *gc.C) {
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

	s.BlockDestroyEnvironment(c, "TestBlockDestroyDestroySubordinateUnits")
	// Try to destroy the subordinate alone; check it fails.
	err = s.APIState.Client().DestroyServiceUnits("logging/0")
	c.Assert(err, gc.ErrorMatches, `no units were destroyed: unit "logging/0" is a subordinate`)
	assertLife(c, logging0, state.Alive)

	s.assertDestroySubordinateUnits(c, wordpress0, logging0)
}

func (s *clientSuite) TestBlockRemoveDestroyRelation(c *gc.C) {
	endpoints := []string{"wordpress", "mysql"}
	relation := s.setupRelationScenario(c, endpoints)
	// block remove-objects
	s.BlockRemoveObject(c, "TestBlockRemoveDestroyRelation")
	err := s.APIState.Client().DestroyRelation(endpoints...)
	s.AssertBlocked(c, err, "TestBlockRemoveDestroyRelation")
	assertLife(c, relation, state.Alive)
}

func (s *clientSuite) TestBlockChangeDestroyRelation(c *gc.C) {
	endpoints := []string{"wordpress", "mysql"}
	relation := s.setupRelationScenario(c, endpoints)
	s.BlockAllChanges(c, "TestBlockChangeDestroyRelation")
	err := s.APIState.Client().DestroyRelation(endpoints...)
	s.AssertBlocked(c, err, "TestBlockChangeDestroyRelation")
	assertLife(c, relation, state.Alive)
}

func (s *clientSuite) TestBlockDestroyDestroyRelation(c *gc.C) {
	s.BlockDestroyEnvironment(c, "TestBlockDestroyDestroyRelation")
	endpoints := []string{"wordpress", "mysql"}
	s.assertDestroyRelation(c, endpoints)
}

func (s *clientSuite) TestDestroyEnvironment(c *gc.C) {
	// The full tests for DestroyEnvironment are in environmentmanager.
	// Here we just test that things are hooked up such that we can destroy
	// the environment through the client endpoint to support older juju clients.
	err := s.APIState.Client().DestroyEnvironment()
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}
