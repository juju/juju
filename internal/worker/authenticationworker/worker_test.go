// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker_test

import (
	"strings"
	"time"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/ssh"
	sshtesting "github.com/juju/utils/v3/ssh/testing"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/worker/authenticationworker"
	"github.com/juju/juju/internal/worker/authenticationworker/mocks"
	coretesting "github.com/juju/juju/testing"
)

type workerSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite

	existingEnvKey    string
	existingModelKeys []string
	existingKeys      []string
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	// Default ssh user is currently "ubuntu".
	c.Assert(authenticationworker.SSHUser, gc.Equals, "ubuntu")
	// Set the ssh user to empty (the current user) as required by the test infrastructure.
	s.PatchValue(&authenticationworker.SSHUser, "")

	// Replace the default dummy key in the test environment with a valid one.
	// This will be added to the ssh authorised keys when the agent starts.
	s.existingModelKeys = []string{sshtesting.ValidKeyOne.Key + " firstuser@host"}

	// Record the existing key with its prefix for testing later.
	s.existingEnvKey = sshtesting.ValidKeyOne.Key + " Juju:firstuser@host"

	// Set up an existing key (which is not in the environment) in the ssh authorised_keys file.
	s.existingKeys = []string{sshtesting.ValidKeyTwo.Key + " existinguser@host"}
	err := ssh.AddKeys(authenticationworker.SSHUser, s.existingKeys...)
	c.Assert(err, jc.ErrorIsNil)
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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)
	client.EXPECT().AuthorisedKeys(tag).Return(s.existingModelKeys, nil)
	client.EXPECT().WatchAuthorisedKeys(tag).Return(watch, nil)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, names.NewMachineTag("666")))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, authWorker)

	newKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:user@host"
	client.EXPECT().AuthorisedKeys(tag).Return([]string{newKeyWithCommentPrefix}, nil)

	ch <- struct{}{}

	s.waitSSHKeys(c, append(s.existingKeys, newKeyWithCommentPrefix))
}

func (s *workerSuite) TestNewKeysInJujuAreSavedOnStartup(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	existingKey := sshtesting.ValidKeyThree.Key + " user@host"

	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)
	client.EXPECT().AuthorisedKeys(tag).Return([]string{existingKey}, nil)
	client.EXPECT().WatchAuthorisedKeys(tag).Return(watch, nil)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, names.NewMachineTag("666")))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, authWorker)

	newKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:user@host"
	client.EXPECT().AuthorisedKeys(tag).Return([]string{newKeyWithCommentPrefix}, nil)

	ch <- struct{}{}

	s.waitSSHKeys(c, append(s.existingKeys, newKeyWithCommentPrefix))
}

func (s *workerSuite) TestDeleteKey(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	// Add another key
	anotherKey := sshtesting.ValidKeyThree.Key + " another@host"

	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)
	client.EXPECT().AuthorisedKeys(tag).Return(append(s.existingModelKeys, anotherKey), nil).Times(2)
	client.EXPECT().WatchAuthorisedKeys(tag).Return(watch, nil)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, names.NewMachineTag("666")))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, authWorker)

	ch <- struct{}{}

	anotherKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:another@host"
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey, anotherKeyWithCommentPrefix))

	// Delete the original key and check anotherKey plus the existing keys remain.
	client.EXPECT().AuthorisedKeys(tag).Return([]string{anotherKey}, nil)
	ch <- struct{}{}

	s.waitSSHKeys(c, append(s.existingKeys, anotherKeyWithCommentPrefix))
}

func (s *workerSuite) TestMultipleChanges(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)
	client.EXPECT().AuthorisedKeys(tag).Return(s.existingModelKeys, nil)
	client.EXPECT().WatchAuthorisedKeys(tag).Return(watch, nil)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, names.NewMachineTag("666")))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, authWorker)

	// Perform a set to add a key and delete a key.
	// added: key 3
	// deleted: key 1 (existing env key)
	yetAnotherKeyWithComment := sshtesting.ValidKeyThree.Key + " Juju:yetanother@host"
	client.EXPECT().AuthorisedKeys(tag).Return([]string{yetAnotherKeyWithComment}, nil)

	ch <- struct{}{}

	s.waitSSHKeys(c, append(s.existingKeys, yetAnotherKeyWithComment))
}

func (s *workerSuite) TestWorkerRestart(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)

	client.EXPECT().WatchAuthorisedKeys(tag).Return(watch, nil).MinTimes(1)

	yetAnotherKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:yetanother@host"

	gomock.InOrder(
		client.EXPECT().AuthorisedKeys(tag).Return(s.existingModelKeys, nil).Times(2),
		client.EXPECT().AuthorisedKeys(tag).Return([]string{yetAnotherKeyWithCommentPrefix}, nil),
	)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, names.NewMachineTag("666")))
	c.Assert(err, jc.ErrorIsNil)
	defer authWorker.Kill()

	// Stop the worker and delete and add keys from the environment while it is down.
	// added: key 3
	// deleted: key 1 (existing env key)
	workertest.CleanKill(c, authWorker)

	authWorker, err = authenticationworker.NewWorker(client, agentConfig(c, names.NewMachineTag("666")))
	c.Assert(err, jc.ErrorIsNil)

	ch <- struct{}{}

	s.waitSSHKeys(c, append(s.existingKeys, yetAnotherKeyWithCommentPrefix))

	workertest.CleanKill(c, authWorker)
}
