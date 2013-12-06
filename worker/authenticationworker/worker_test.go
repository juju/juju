// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker_test

import (
	"strings"
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/credentials"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/ssh"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/authenticationworker"
)

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = 5 * time.Second

var validKey = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDEX/dPu4PmtvgK3La9zioCEDrJ` +
	`yUr6xEIK7Pr+rLgydcqWTU/kt7w7gKjOw4vvzgHfjKl09CWyvgb+y5dCiTk` +
	`9MxI+erGNhs3pwaoS+EavAbawB7iEqYyTep3YaJK+4RJ4OX7ZlXMAIMrTL+` +
	`UVrK89t56hCkFYaAgo3VY+z6rb/b3bDBYtE1Y2tS7C3au73aDgeb9psIrSV` +
	`86ucKBTl5X62FnYiyGd++xCnLB6uLximM5OKXfLzJQNS/QyZyk12g3D8y69` +
	`Xw1GzCSKX1u1+MQboyf0HJcG2ryUCLHdcDVppApyHx2OLq53hlkQ/yxdflD` +
	`qCqAE4j+doagSsIfC1T2T`

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var _ = gc.Suite(&workerSuite{})

type workerSuite struct {
	jujutesting.JujuConnSuite
	stateMachine   *state.Machine
	machine        *state.Machine
	credentialsApi *credentials.State

	existingKeys []string
}

func (s *workerSuite) SetUpTest(c *gc.C) {
	fakeHome := coretesting.MakeEmptyFakeHomeWithoutJuju(c)
	s.AddCleanup(func(*gc.C) { fakeHome.Restore() })

	s.JujuConnSuite.SetUpTest(c)

	// Replace the default dummy key in the test environment with a valid one.
	s.setAuthorisedKeys(c, validKey+" firstuser@host")

	// Set up an existing key in the ssh authorised_keys file
	s.existingKeys = []string{validKey + " existinguser@host"}
	err := ssh.AddKeys(s.existingKeys...)
	c.Assert(err, gc.IsNil)

	s.machine, err = s.BackingState.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	s.stateMachine, err = s.BackingState.AddMachine("quantal", state.JobManageState, state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = s.stateMachine.SetPassword(password)
	c.Assert(err, gc.IsNil)
	err = s.stateMachine.SetProvisioned("i-manager", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	apiRoot := s.OpenAPIAsMachine(c, s.stateMachine.Tag(), password, "fake_nonce")
	c.Assert(apiRoot, gc.NotNil)
	s.credentialsApi = apiRoot.Credentials()
	c.Assert(s.credentialsApi, gc.NotNil)
}

func stop(c *gc.C, w worker.Worker) {
	c.Assert(worker.Stop(w), gc.IsNil)
}

type mockConfig struct {
	agent.Config
	c   *gc.C
	tag string
}

func (mock *mockConfig) Tag() string {
	return mock.tag
}

func agentConfig(c *gc.C, tag string) *mockConfig {
	return &mockConfig{c: c, tag: tag}
}

func (s *workerSuite) setAuthorisedKeys(c *gc.C, keys ...string) {
	keyStr := strings.Join(keys, "\n")
	err := testing.UpdateConfig(s.BackingState, map[string]interface{}{"authorized-keys": keyStr})
	c.Assert(err, gc.IsNil)
	s.BackingState.StartSync()
}

func (s *workerSuite) waitSSHKeys(c *gc.C, expected []string) {
	timeout := time.After(worstCase)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for authoirsed ssh keys to change")
		case <-time.After(10 * time.Millisecond):
			keys, err := ssh.ListKeys(ssh.FullKeys)
			c.Assert(err, gc.IsNil)
			keysStr := strings.Join(keys, "\n")
			expectedStr := strings.Join(expected, "\n")
			if expectedStr != keysStr {
				continue
			}
			return
		}
	}
}

func (s *workerSuite) TestKeyUpdateRetainsExisting(c *gc.C) {
	authWorker := authenticationworker.NewWorker(s.credentialsApi, agentConfig(c, s.machine.Tag()))
	defer stop(c, authWorker)

	newKey := validKey + " user@host"
	s.setAuthorisedKeys(c, newKey)
	s.waitSSHKeys(c, append(s.existingKeys, newKey))
}

func (s *workerSuite) TestNewKeysInJujuAreSavedOnStartup(c *gc.C) {
	newKey := validKey + " user@host"
	s.setAuthorisedKeys(c, newKey)

	authWorker := authenticationworker.NewWorker(s.credentialsApi, agentConfig(c, s.machine.Tag()))
	defer stop(c, authWorker)

	s.waitSSHKeys(c, append(s.existingKeys, newKey))
}

func (s *workerSuite) TestDeleteKey(c *gc.C) {
	newKey := validKey + " user@host"
	s.setAuthorisedKeys(c, newKey)

	authWorker := authenticationworker.NewWorker(s.credentialsApi, agentConfig(c, s.machine.Tag()))
	defer stop(c, authWorker)

	// Add another key
	anotherKey := validKey + " another@host"
	s.setAuthorisedKeys(c, newKey, anotherKey)
	s.waitSSHKeys(c, append(s.existingKeys, newKey, anotherKey))

	// Delete the original key and check anotherKey plus the existing keys remain.
	s.setAuthorisedKeys(c, anotherKey)
	s.waitSSHKeys(c, append(s.existingKeys, anotherKey))
}

func (s *workerSuite) TestMultipleChanges(c *gc.C) {
	authWorker := authenticationworker.NewWorker(s.credentialsApi, agentConfig(c, s.machine.Tag()))
	defer stop(c, authWorker)

	newKey := validKey + " user@host"
	anotherKey := validKey + " another@host"
	s.setAuthorisedKeys(c, newKey, anotherKey)
	s.waitSSHKeys(c, append(s.existingKeys, newKey, anotherKey))

	// Add a key and delete a key.
	yetAnotherKey := validKey + " yetanother@host"
	s.setAuthorisedKeys(c, newKey, yetAnotherKey)
	s.waitSSHKeys(c, append(s.existingKeys, newKey, yetAnotherKey))
}
