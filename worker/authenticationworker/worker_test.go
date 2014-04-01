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
	s.JujuConnSuite.SetUpTest(c)
	// Default ssh user is currently "ubuntu".
	c.Assert(authenticationworker.SSHUser, gc.Equals, "ubuntu")
	// Set the ssh user to empty (the current user) as required by the test infrastructure.
	s.PatchValue(&authenticationworker.SSHUser, "")

	// Replace the default dummy key in the test environment with a valid one.
	// This will be added to the ssh authorised keys when the agent starts.
	s.setAuthorisedKeys(c, sshtesting.ValidKeyOne.Key+" firstuser@host")
	// Record the existing key with its prefix for testing later.
	s.existingEnvKey = sshtesting.ValidKeyOne.Key + " Juju:firstuser@host"

	// Set up an existing key (which is not in the environment) in the ssh authorised_keys file.
	s.existingKeys = []string{sshtesting.ValidKeyTwo.Key + " existinguser@host"}
	err := ssh.AddKeys(authenticationworker.SSHUser, s.existingKeys...)
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
	err := s.BackingState.UpdateEnvironConfig(map[string]interface{}{"authorized-keys": keyStr}, nil, nil)
	c.Assert(err, gc.IsNil)
	s.BackingState.StartSync()
}

func (s *workerSuite) waitSSHKeys(c *gc.C, expected []string) {
	timeout := time.After(worstCase)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for authoirsed ssh keys to change")
		case <-time.After(coretesting.ShortWait):
			keys, err := ssh.ListKeys(authenticationworker.SSHUser, ssh.FullKeys)
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
	newKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:user@host"
	s.waitSSHKeys(c, append(s.existingKeys, newKeyWithCommentPrefix))
}

func (s *workerSuite) TestNewKeysInJujuAreSavedOnStartup(c *gc.C) {
	newKey := sshtesting.ValidKeyThree.Key + " user@host"
	s.setAuthorisedKeys(c, newKey)

	authWorker := authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag()))
	defer stop(c, authWorker)

	newKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:user@host"
	s.waitSSHKeys(c, append(s.existingKeys, newKeyWithCommentPrefix))
}

func (s *workerSuite) TestDeleteKey(c *gc.C) {
	authWorker := authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag()))
	defer stop(c, authWorker)

	// Add another key
	anotherKey := sshtesting.ValidKeyThree.Key + " another@host"
	s.setAuthorisedKeys(c, s.existingEnvKey, anotherKey)
	anotherKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:another@host"
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey, anotherKeyWithCommentPrefix))

	// Delete the original key and check anotherKey plus the existing keys remain.
	s.setAuthorisedKeys(c, anotherKey)
	s.waitSSHKeys(c, append(s.existingKeys, anotherKeyWithCommentPrefix))
}

func (s *workerSuite) TestMultipleChanges(c *gc.C) {
	authWorker := authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag()))
	defer stop(c, authWorker)
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey))

	// Perform a set to add a key and delete a key.
	// added: key 3
	// deleted: key 1 (existing env key)
	s.setAuthorisedKeys(c, sshtesting.ValidKeyThree.Key+" yetanother@host")
	yetAnotherKeyWithComment := sshtesting.ValidKeyThree.Key + " Juju:yetanother@host"
	s.waitSSHKeys(c, append(s.existingKeys, yetAnotherKeyWithComment))
}

func (s *workerSuite) TestWorkerRestart(c *gc.C) {
	authWorker := authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag()))
	defer stop(c, authWorker)
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey))

	// Stop the worker and delete and add keys from the environment while it is down.
	// added: key 3
	// deleted: key 1 (existing env key)
	stop(c, authWorker)
	s.setAuthorisedKeys(c, sshtesting.ValidKeyThree.Key+" yetanother@host")

	// Restart the worker and check that the ssh auth keys are as expected.
	authWorker = authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag()))
	defer stop(c, authWorker)

	yetAnotherKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:yetanother@host"
	s.waitSSHKeys(c, append(s.existingKeys, yetAnotherKeyWithCommentPrefix))
}
