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
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/keyupdater"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/ssh"
	sshtesting "launchpad.net/juju-core/utils/ssh/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/authenticationworker"
)

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = 5 * time.Second

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var _ = gc.Suite(&workerSuite{})

type workerSuite struct {
	jujutesting.JujuConnSuite
	stateMachine  *state.Machine
	machine       *state.Machine
	keyupdaterApi *keyupdater.State

	existingEnvKey string
	existingKeys   []string
}

func (s *workerSuite) SetUpTest(c *gc.C) {
	fakeHome := coretesting.MakeEmptyFakeHomeWithoutJuju(c)
	s.AddCleanup(func(*gc.C) { fakeHome.Restore() })

	s.JujuConnSuite.SetUpTest(c)

	// Replace the default dummy key in the test environment with a valid one.
	s.existingEnvKey = sshtesting.ValidKeyOne.Key + " firstuser@host"
	s.setAuthorisedKeys(c, s.existingEnvKey)

	// Set up an existing key (which is not in the environment) in the ssh authorised_keys file.
	s.existingKeys = []string{sshtesting.ValidKeyTwo.Key + " existinguser@host"}
	err := ssh.AddKeys(s.existingKeys...)
	c.Assert(err, gc.IsNil)

	var apiRoot *api.State
	apiRoot, s.machine = s.OpenAPIAsNewMachine(c)
	c.Assert(apiRoot, gc.NotNil)
	s.keyupdaterApi = apiRoot.KeyUpdater()
	c.Assert(s.keyupdaterApi, gc.NotNil)
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
	authWorker := authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag()))
	defer stop(c, authWorker)

	newKey := sshtesting.ValidKeyThree.Key + " user@host"
	s.setAuthorisedKeys(c, newKey)
	s.waitSSHKeys(c, append(s.existingKeys, newKey))
}

func (s *workerSuite) TestNewKeysInJujuAreSavedOnStartup(c *gc.C) {
	newKey := sshtesting.ValidKeyThree.Key + " user@host"
	s.setAuthorisedKeys(c, newKey)

	authWorker := authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag()))
	defer stop(c, authWorker)

	s.waitSSHKeys(c, append(s.existingKeys, newKey))
}

func (s *workerSuite) TestDeleteKey(c *gc.C) {
	authWorker := authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag()))
	defer stop(c, authWorker)

	// Add another key
	anotherKey := sshtesting.ValidKeyThree.Key + " another@host"
	s.setAuthorisedKeys(c, s.existingEnvKey, anotherKey)
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey, anotherKey))

	// Delete the original key and check anotherKey plus the existing keys remain.
	s.setAuthorisedKeys(c, anotherKey)
	s.waitSSHKeys(c, append(s.existingKeys, anotherKey))
}

func (s *workerSuite) TestMultipleChanges(c *gc.C) {
	authWorker := authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag()))
	defer stop(c, authWorker)
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey))

	// Perform a set to add a key and delete a key.
	// added: key 3
	// deleted: key 1 (existing env key)
	yetAnotherKey := sshtesting.ValidKeyThree.Key + " yetanother@host"
	s.setAuthorisedKeys(c, yetAnotherKey)
	s.waitSSHKeys(c, append(s.existingKeys, yetAnotherKey))
}
