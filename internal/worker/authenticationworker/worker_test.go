// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/ssh"
	sshtesting "github.com/juju/utils/v4/ssh/testing"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/agent"
	coremachineauthentication "github.com/juju/juju/core/machineauthentication"
	coressh "github.com/juju/juju/core/ssh"
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

// waitSSHKeysUnordered is like waitSSHKeys but compares the authorised keys as
// a set rather than an ordered list. It is used where the final ordering of
// keys in the file depends on concurrent operations interleaving, but the set
// of keys present is deterministic. Ordering in authorized_keys is not
// significant.
func (s *workerSuite) waitSSHKeysUnordered(c *tc.C, expected []string) {
	expectedSet := set.NewStrings(expected...)
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for authorised ssh keys to change")
		case <-time.After(coretesting.ShortWait):
			keys, err := ssh.ListKeys(authenticationworker.SSHUser, ssh.FullKeys)
			c.Assert(err, tc.ErrorIsNil)
			if set.NewStrings(keys...).Difference(expectedSet).IsEmpty() &&
				expectedSet.Difference(set.NewStrings(keys...)).IsEmpty() {
				return
			}
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

	updater, ok := authWorker.(coressh.EphemeralKeysUpdater)
	c.Assert(ok, tc.IsTrue)

	pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(sshtesting.ValidKeyThree.Key))
	c.Assert(err, tc.ErrorIsNil)

	// Adding the ephemeral key writes it into the authorized_keys file, tagged
	// with the ephemeral comment prefix.
	err = updater.AddEphemeralKey(pub, "tunnel-0")
	c.Assert(err, tc.ErrorIsNil)

	ephemeralKey := sshtesting.ValidKeyThree.Key + " Juju:Ephemeral:tunnel-0"
	s.waitSSHKeysUnordered(c, append(s.existingKeys, s.existingEnvKey, ephemeralKey))

	// Removing the ephemeral key drops it again.
	err = updater.RemoveEphemeralKey(pub)
	c.Assert(err, tc.ErrorIsNil)

	s.waitSSHKeysUnordered(c, append(s.existingKeys, s.existingEnvKey))
}

// TestRemoveLastEphemeralKey asserts that removing an ephemeral key succeeds
// even when it is the only key in the authorized_keys file. The underlying
// DeleteKeys helper refuses to delete the final key, so the worker must use
// the delete-all variant; otherwise a torn-down tunnel's key would remain
// authorised until the worker restarts.
func (s *workerSuite) TestRemoveLastEphemeralKey(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Clear the authorized_keys file (SetUpTest seeds one existing key) and
	// start a worker with no model keys, so the ephemeral key becomes the only
	// key present.
	err := ssh.DeleteKeysFromFile(authenticationworker.SSHUser, "authorized_keys", []string{"existinguser@host"})
	c.Assert(err, tc.ErrorIsNil)

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)
	client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return([]string{}, nil).AnyTimes()
	client.EXPECT().WatchAuthorisedKeys(gomock.Any(), tag).Return(watch, nil)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, tag))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, authWorker)
	s.waitSSHKeysUnordered(c, nil)

	updater, ok := authWorker.(coressh.EphemeralKeysUpdater)
	c.Assert(ok, tc.IsTrue)

	pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(sshtesting.ValidKeyThree.Key))
	c.Assert(err, tc.ErrorIsNil)

	err = updater.AddEphemeralKey(pub, "tunnel-0")
	c.Assert(err, tc.ErrorIsNil)
	ephemeralKey := sshtesting.ValidKeyThree.Key + " Juju:Ephemeral:tunnel-0"
	s.waitSSHKeysUnordered(c, []string{ephemeralKey})

	// Removing the only key must succeed, leaving an empty key set.
	err = updater.RemoveEphemeralKey(pub)
	c.Assert(err, tc.ErrorIsNil)
	s.waitSSHKeysUnordered(c, nil)
}

// TestKillDuringModelKeyUpdateReportsCleanShutdown asserts that killing the
// worker while a model key update is in flight is reported as a normal
// shutdown. A Kill cancels the catacomb context, which the in-flight
// AuthorisedKeys call surfaces as a cancelled-context error; the worker must
// translate that into ErrDying rather than reporting a failure to the runner.
func (s *workerSuite) TestKillDuringModelKeyUpdateReportsCleanShutdown(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)
	client.EXPECT().WatchAuthorisedKeys(gomock.Any(), tag).Return(watch, nil)

	// blocked signals that the model key update is in flight; release lets it
	// return once the worker has been killed.
	blocked := make(chan struct{})
	release := make(chan struct{})
	gomock.InOrder(
		// The initial setUp synchronisation.
		client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return(s.existingModelKeys, nil),
		// The model key update triggered by the watcher: block until killed,
		// then return the cancelled-context error the real client would.
		client.EXPECT().AuthorisedKeys(gomock.Any(), tag).DoAndReturn(
			func(ctx context.Context, _ names.MachineTag) ([]string, error) {
				close(blocked)
				<-release
				return nil, ctx.Err()
			},
		),
	)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, tag))
	c.Assert(err, tc.ErrorIsNil)
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey))

	// Fire the watcher and wait for the model key update to be in flight.
	ch <- struct{}{}
	select {
	case <-blocked:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for model key update to start")
	}

	// Kill the worker (cancelling the context), then release the API call so it
	// returns the cancelled-context error.
	authWorker.Kill()
	close(release)

	// A clean kill must observe ErrDying, not the cancelled-context failure.
	err = workertest.CheckKilled(c, authWorker)
	c.Check(err, tc.ErrorIsNil)
}

func (s *workerSuite) TestEphemeralKeyRemovedOnRestart(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	authWorker := s.startWorker(c, ctrl)

	updater, ok := authWorker.(coressh.EphemeralKeysUpdater)
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

func (s *workerSuite) TestAddEphemeralKeyDuringModelChange(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)

	tag := names.NewMachineTag("666")
	client := mocks.NewMockClient(ctrl)

	// The model key changes to key three on the watcher fire.
	changedModelKey := sshtesting.ValidKeyThree.Key + " yetanother@host"
	gomock.InOrder(
		client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return(s.existingModelKeys, nil),
		client.EXPECT().AuthorisedKeys(gomock.Any(), tag).Return([]string{changedModelKey}, nil),
	)
	client.EXPECT().WatchAuthorisedKeys(gomock.Any(), tag).Return(watch, nil)

	authWorker, err := authenticationworker.NewWorker(client, agentConfig(c, tag))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, authWorker)
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey))

	updater, ok := authWorker.(coressh.EphemeralKeysUpdater)
	c.Assert(ok, tc.IsTrue)

	key4, _, _, _, err := gossh.ParseAuthorizedKey([]byte(sshtesting.ValidKeyFour.Key))
	c.Assert(err, tc.ErrorIsNil)

	// Add an ephemeral key in a goroutine to simulate it racing with a model
	// key change being processed by the worker's Handle. The two writers to
	// authorized_keys must not clobber each other: the final state must contain
	// both the changed model key and the ephemeral key.
	go func() {
		c.Check(updater.AddEphemeralKey(key4, "key4-comment"), tc.ErrorIsNil)
	}()

	// Fire the watcher so the worker processes the model key change. This
	// replaces the original model key (existingEnvKey) with the changed key.
	ch <- struct{}{}

	// The final ordering of keys depends on how AddEphemeralKey interleaves
	// with the model key update, so compare as a set. The invariant under test
	// is that neither write clobbers the other: both keys must be present.
	changedModelKeyWithComment := sshtesting.ValidKeyThree.Key + " Juju:yetanother@host"
	ephemeralKeyWithComment := sshtesting.ValidKeyFour.Key + " Juju:Ephemeral:key4-comment"
	s.waitSSHKeysUnordered(c, append(s.existingKeys, changedModelKeyWithComment, ephemeralKeyWithComment))
}

// TestEphemeralKeyOpsReturnWorkerDying asserts that once the worker is dying,
// both AddEphemeralKey and RemoveEphemeralKey return
// ErrAuthenticationWorkerDying rather than the catacomb's internal dying error,
// so consuming workers can distinguish a lifecycle stop from a real failure.
func (s *workerSuite) TestEphemeralKeyOpsReturnWorkerDying(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	authWorker := s.startWorker(c, ctrl)

	updater, ok := authWorker.(coressh.EphemeralKeysUpdater)
	c.Assert(ok, tc.IsTrue)

	pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(sshtesting.ValidKeyThree.Key))
	c.Assert(err, tc.ErrorIsNil)

	// Stop the worker so that enqueue observes the catacomb dying and any
	// ephemeral key operation short-circuits with the sentinel error.
	workertest.CleanKill(c, authWorker)

	err = updater.AddEphemeralKey(pub, "tunnel-0")
	c.Check(err, tc.ErrorIs, coremachineauthentication.ErrAuthenticationWorkerDying)

	err = updater.RemoveEphemeralKey(pub)
	c.Check(err, tc.ErrorIs, coremachineauthentication.ErrAuthenticationWorkerDying)
}
