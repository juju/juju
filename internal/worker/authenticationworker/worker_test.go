// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker_test

import (
	"strings"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/ssh"
	sshtesting "github.com/juju/utils/v4/ssh/testing"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/authenticationworker"
	"github.com/juju/juju/internal/worker/authenticationworker/mocks"
)

type workerSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite

	existingEnvKey    string
	existingModelKeys []string
	existingKeys      []string
}

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	// Default ssh user is currently "ubuntu".
	c.Assert(authenticationworker.SSHUser, tc.Equals, "ubuntu")
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
	c.Assert(err, tc.ErrorIsNil)
}

type mockConfig struct {
	agent.Config
	c   *tc.C
	tag names.Tag
}

func (mock *mockConfig) Tag() names.Tag {
	return mock.tag
}

func agentConfig(c *tc.C, tag names.MachineTag) *mockConfig {
	return &mockConfig{c: c, tag: tag}
}

func (s *workerSuite) waitSSHKeys(c *tc.C, expected []string) {
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for authoirsed ssh keys to change")
		case <-time.After(coretesting.ShortWait):
			keys, err := ssh.ListKeys(authenticationworker.SSHUser, ssh.FullKeys)
			c.Assert(err, tc.ErrorIsNil)
			keysStr := strings.Join(keys, "\n")
			expectedStr := strings.Join(expected, "\n")
			if expectedStr != keysStr {
				continue
			}
			return
		}
	}
}

func (s *workerSuite) TestKeyUpdateRetainsExisting(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)

	newKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:user@host"
	gomock.InOrder(
		client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return(s.existingModelKeys, nil),
		client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return([]string{newKeyWithCommentPrefix}, nil),
	)
	client.EXPECT().WatchAuthorisedKeys(gomock.Any(), tag).Return(watch, nil)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, names.NewMachineTag("666")))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, authWorker)

	ch <- struct{}{}

	s.waitSSHKeys(c, append(s.existingKeys, newKeyWithCommentPrefix))
}

func (s *workerSuite) TestNewKeysInJujuAreSavedOnStartup(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	existingKey := sshtesting.ValidKeyThree.Key + " user@host"

	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)

	newKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:user@host"
	gomock.InOrder(
		client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return([]string{existingKey}, nil),
		client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return([]string{newKeyWithCommentPrefix}, nil),
	)
	client.EXPECT().WatchAuthorisedKeys(gomock.Any(), tag).Return(watch, nil)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, names.NewMachineTag("666")))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, authWorker)

	ch <- struct{}{}

	s.waitSSHKeys(c, append(s.existingKeys, newKeyWithCommentPrefix))
}

func (s *workerSuite) TestDeleteKey(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	// Add another key
	anotherKey := sshtesting.ValidKeyThree.Key + " another@host"

	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)
	client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return(append(s.existingModelKeys, anotherKey), nil).Times(2)
	client.EXPECT().WatchAuthorisedKeys(gomock.Any(), tag).Return(watch, nil)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, names.NewMachineTag("666")))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, authWorker)

	ch <- struct{}{}

	anotherKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:another@host"
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey, anotherKeyWithCommentPrefix))

	// Delete the original key and check anotherKey plus the existing keys remain.
	client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return([]string{anotherKey}, nil)
	ch <- struct{}{}

	s.waitSSHKeys(c, append(s.existingKeys, anotherKeyWithCommentPrefix))
}

func (s *workerSuite) TestMultipleChanges(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)

	// Perform a set to add a key and delete a key.
	// added: key 3
	// deleted: key 1 (existing env key)
	yetAnotherKeyWithComment := sshtesting.ValidKeyThree.Key + " Juju:yetanother@host"
	gomock.InOrder(
		client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return(s.existingModelKeys, nil),
		client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return([]string{yetAnotherKeyWithComment}, nil),
	)
	client.EXPECT().WatchAuthorisedKeys(gomock.Any(), tag).Return(watch, nil)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, names.NewMachineTag("666")))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, authWorker)

	ch <- struct{}{}

	s.waitSSHKeys(c, append(s.existingKeys, yetAnotherKeyWithComment))
}

func (s *workerSuite) TestWorkerRestart(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)

	client.EXPECT().WatchAuthorisedKeys(gomock.Any(), tag).Return(watch, nil).MinTimes(1)

	yetAnotherKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:yetanother@host"

	gomock.InOrder(
		client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return(s.existingModelKeys, nil).Times(2),
		client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return([]string{yetAnotherKeyWithCommentPrefix}, nil),
	)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, names.NewMachineTag("666")))
	c.Assert(err, tc.ErrorIsNil)
	defer authWorker.Kill()

	// Stop the worker and delete and add keys from the environment while it is down.
	// added: key 3
	// deleted: key 1 (existing env key)
	workertest.CleanKill(c, authWorker)

	authWorker, err = authenticationworker.NewWorker(client, agentConfig(c, names.NewMachineTag("666")))
	c.Assert(err, tc.ErrorIsNil)

	ch <- struct{}{}

	s.waitSSHKeys(c, append(s.existingKeys, yetAnotherKeyWithCommentPrefix))

	workertest.CleanKill(c, authWorker)
}

// startWorker starts an auth worker that only reports the existing model keys,
// and returns it. The watcher never fires so the authorized_keys file settles
// to the existing keys plus the model key.
func (s *workerSuite) startWorker(c *tc.C, ctrl *gomock.Controller) worker.Worker {
	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)
	client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return(s.existingModelKeys, nil).AnyTimes()
	client.EXPECT().WatchAuthorisedKeys(gomock.Any(), tag).Return(watch, nil)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, tag))
	c.Assert(err, tc.ErrorIsNil)
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey))
	return authWorker
}

func (s *workerSuite) TestAddAndRemoveEphemeralKey(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	authWorker := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, authWorker)

	updater, ok := authWorker.(authenticationworker.EphemeralKeysUpdater)
	c.Assert(ok, tc.IsTrue)

	pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(sshtesting.ValidKeyThree.Key))
	c.Assert(err, tc.ErrorIsNil)

	// Adding the ephemeral key writes it into the authorized_keys file, tagged
	// with the ephemeral comment prefix.
	err = updater.AddEphemeralKey(pub, "tunnel-0")
	c.Assert(err, tc.ErrorIsNil)

	ephemeralKey := sshtesting.ValidKeyThree.Key + " Juju:Ephemeral:tunnel-0"
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey, ephemeralKey))

	// Removing the ephemeral key drops it again.
	err = updater.RemoveEphemeralKey(pub)
	c.Assert(err, tc.ErrorIsNil)

	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey))
}

func (s *workerSuite) TestEphemeralKeyRemovedOnRestart(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	authWorker := s.startWorker(c, ctrl)

	updater, ok := authWorker.(authenticationworker.EphemeralKeysUpdater)
	c.Assert(ok, tc.IsTrue)

	pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(sshtesting.ValidKeyThree.Key))
	c.Assert(err, tc.ErrorIsNil)
	err = updater.AddEphemeralKey(pub, "tunnel-0")
	c.Assert(err, tc.ErrorIsNil)

	ephemeralKey := sshtesting.ValidKeyThree.Key + " Juju:Ephemeral:tunnel-0"
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey, ephemeralKey))

	// Stop the worker, leaving the ephemeral key dangling in authorized_keys.
	workertest.CleanKill(c, authWorker)

	// A fresh worker must strip any dangling ephemeral keys on startup, since
	// they carry the Juju comment prefix and are not part of the model keys.
	authWorker = s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, authWorker)

	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey))
}
