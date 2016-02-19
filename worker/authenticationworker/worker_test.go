// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker_test

import (
	"runtime"
	"strings"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/ssh"
	sshtesting "github.com/juju/utils/ssh/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/keyupdater"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/authenticationworker"
)

type workerSuite struct {
	jujutesting.JujuConnSuite
	stateMachine  *state.Machine
	machine       *state.Machine
	keyupdaterApi *keyupdater.State

	existingEnvKey string
	existingKeys   []string
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	//TODO(bogdanteleaga): Fix this on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: authentication worker not implemented yet on windows")
	}
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
	c.Assert(err, jc.ErrorIsNil)

	var apiRoot api.Connection
	apiRoot, s.machine = s.OpenAPIAsNewMachine(c)
	c.Assert(apiRoot, gc.NotNil)
	s.keyupdaterApi = keyupdater.NewState(apiRoot)
	c.Assert(s.keyupdaterApi, gc.NotNil)
}

func stop(c *gc.C, w worker.Worker) {
	c.Assert(worker.Stop(w), gc.IsNil)
}

type mockConfig struct {
	agent.Config
	c   *gc.C
	tag names.Tag
}

func (mock *mockConfig) Tag() names.Tag {
	return mock.tag
}

func agentConfig(c *gc.C, tag names.MachineTag) *mockConfig {
	return &mockConfig{c: c, tag: tag}
}

func (s *workerSuite) setAuthorisedKeys(c *gc.C, keys ...string) {
	keyStr := strings.Join(keys, "\n")
	err := s.BackingState.UpdateModelConfig(map[string]interface{}{"authorized-keys": keyStr}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.BackingState.StartSync()
}

func (s *workerSuite) waitSSHKeys(c *gc.C, expected []string) {
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for authoirsed ssh keys to change")
		case <-time.After(coretesting.ShortWait):
			keys, err := ssh.ListKeys(authenticationworker.SSHUser, ssh.FullKeys)
			c.Assert(err, jc.ErrorIsNil)
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
	authWorker, err := authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag().(names.MachineTag)))
	c.Assert(err, jc.ErrorIsNil)
	defer stop(c, authWorker)

	newKey := sshtesting.ValidKeyThree.Key + " user@host"
	s.setAuthorisedKeys(c, newKey)
	newKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:user@host"
	s.waitSSHKeys(c, append(s.existingKeys, newKeyWithCommentPrefix))
}

func (s *workerSuite) TestNewKeysInJujuAreSavedOnStartup(c *gc.C) {
	newKey := sshtesting.ValidKeyThree.Key + " user@host"
	s.setAuthorisedKeys(c, newKey)

	authWorker, err := authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag().(names.MachineTag)))
	c.Assert(err, jc.ErrorIsNil)
	defer stop(c, authWorker)

	newKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:user@host"
	s.waitSSHKeys(c, append(s.existingKeys, newKeyWithCommentPrefix))
}

func (s *workerSuite) TestDeleteKey(c *gc.C) {
	authWorker, err := authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag().(names.MachineTag)))
	c.Assert(err, jc.ErrorIsNil)
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
	authWorker, err := authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag().(names.MachineTag)))
	c.Assert(err, jc.ErrorIsNil)
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
	authWorker, err := authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag().(names.MachineTag)))
	c.Assert(err, jc.ErrorIsNil)
	defer stop(c, authWorker)
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey))

	// Stop the worker and delete and add keys from the environment while it is down.
	// added: key 3
	// deleted: key 1 (existing env key)
	stop(c, authWorker)
	s.setAuthorisedKeys(c, sshtesting.ValidKeyThree.Key+" yetanother@host")

	// Restart the worker and check that the ssh auth keys are as expected.
	authWorker, err = authenticationworker.NewWorker(s.keyupdaterApi, agentConfig(c, s.machine.Tag().(names.MachineTag)))
	c.Assert(err, jc.ErrorIsNil)
	defer stop(c, authWorker)

	yetAnotherKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:yetanother@host"
	s.waitSSHKeys(c, append(s.existingKeys, yetAnotherKeyWithCommentPrefix))
}
