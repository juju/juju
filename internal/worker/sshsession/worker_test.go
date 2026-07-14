// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	gossh "golang.org/x/crypto/ssh"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type workerSuite struct {
	testhelpers.IsolationSuite

	facadeClient *MockFacadeClient
	keysUpdater  *MockEphemeralKeysUpdater
	dialer       *MockConnectionDialer
}

func TestWorkerSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &workerSuite{})
	})
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.facadeClient = NewMockFacadeClient(ctrl)
	s.keysUpdater = NewMockEphemeralKeysUpdater(ctrl)
	s.dialer = NewMockConnectionDialer(ctrl)
	return ctrl
}

func (s *workerSuite) newConfig(c *tc.C, machineName string) WorkerConfig {
	return WorkerConfig{
		Logger:               loggertesting.WrapCheckLog(c),
		MachineName:          machineName,
		FacadeClient:         s.facadeClient,
		EphemeralKeysUpdater: s.keysUpdater,
		ConnectionDialer:     s.dialer,
	}
}

func (s *workerSuite) TestValidate(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg := s.newConfig(c, "0")
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.newConfig(c, "0")
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, coreerrors.NotValid)

	cfg = s.newConfig(c, "")
	c.Check(cfg.Validate(), tc.ErrorIs, coreerrors.NotValid)

	cfg = s.newConfig(c, "0")
	cfg.FacadeClient = nil
	c.Check(cfg.Validate(), tc.ErrorIs, coreerrors.NotValid)

	cfg = s.newConfig(c, "0")
	cfg.EphemeralKeysUpdater = nil
	c.Check(cfg.Validate(), tc.ErrorIs, coreerrors.NotValid)

	cfg = s.newConfig(c, "0")
	cfg.ConnectionDialer = nil
	c.Check(cfg.Validate(), tc.ErrorIs, coreerrors.NotValid)
}

// newPublicKey returns a fresh marshalled ed25519 SSH public key.
func newPublicKey(c *tc.C) []byte {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	c.Assert(err, tc.ErrorIsNil)
	sshPub, err := gossh.NewPublicKey(pub)
	c.Assert(err, tc.ErrorIsNil)
	return sshPub.Marshal()
}

func (s *workerSuite) startWorker(c *tc.C, machineName string, changes chan []string) (*sshSessionWorker, *watchertest.MockStringsWatcher) {
	w := watchertest.NewMockStringsWatcher(changes)
	s.facadeClient.EXPECT().WatchSSHConnRequest(gomock.Any()).Return(w, nil)
	// The worker fetches the controller SSH port and host public key once at
	// startup.
	s.facadeClient.EXPECT().ControllerSSHPort(gomock.Any()).Return(2223, nil)
	s.facadeClient.EXPECT().ControllerPublicKey(gomock.Any()).Return(newPublicKey(c), nil)

	worker, err := NewWorker(s.newConfig(c, machineName))
	c.Assert(err, tc.ErrorIsNil)
	return worker.(*sshSessionWorker), w
}

func (s *workerSuite) TestHandlesRequestForOwnMachine(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	ephemeralKey := newPublicKey(c)

	req := params.SSHConnRequestResult{
		MachineName:         "0",
		ControllerAddresses: []string{"10.0.0.1"},
		Username:            "juju-reverse-tunnel",
		Password:            "jwt",
		EphemeralPublicKey:  ephemeralKey,
	}

	handled := make(chan struct{})
	s.facadeClient.EXPECT().GetSSHConnRequest(gomock.Any(), "tunnel-0").Return(req, nil)
	s.keysUpdater.EXPECT().AddEphemeralKey(gomock.Any(), "tunnel-0").Return(nil)

	controllerConn, controllerRemote := net.Pipe()
	sshdConn, sshdRemote := net.Pipe()
	s.dialer.EXPECT().DialController(gomock.Any(), "10.0.0.1", 2223, "juju-reverse-tunnel", "jwt", gomock.Any()).Return(controllerConn, nil)
	s.dialer.EXPECT().DialLocalSSHD(gomock.Any()).Return(sshdConn, nil)
	s.keysUpdater.EXPECT().RemoveEphemeralKey(gomock.Any()).DoAndReturn(func(gossh.PublicKey) error {
		close(handled)
		return nil
	})

	changes := make(chan []string)
	worker, _ := s.startWorker(c, "0", changes)
	defer workertest.DirtyKill(c, worker)

	select {
	case changes <- []string{"tunnel-0"}:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out sending change")
	}

	// Close the remote ends so the worker's bidirectional io.Copy reaches EOF,
	// completing the handler and triggering the deferred RemoveEphemeralKey.
	_ = controllerRemote.Close()
	_ = sshdRemote.Close()

	select {
	case <-handled:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for request to be handled")
	}

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestSkipsRequestForOtherMachine(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	got := make(chan struct{})
	req := params.SSHConnRequestResult{MachineName: "1"}
	s.facadeClient.EXPECT().GetSSHConnRequest(gomock.Any(), "tunnel-1").DoAndReturn(
		func(context.Context, string) (params.SSHConnRequestResult, error) {
			close(got)
			return req, nil
		},
	)
	// No dialer or key-updater calls expected for another machine's request.

	changes := make(chan []string)
	worker, _ := s.startWorker(c, "0", changes)
	defer workertest.DirtyKill(c, worker)

	select {
	case changes <- []string{"tunnel-1"}:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out sending change")
	}

	select {
	case <-got:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for request to be read")
	}

	workertest.CleanKill(c, worker)
}

func (s *workerSuite) TestWatcherClosedStopsWorker(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	changes := make(chan []string)
	worker, _ := s.startWorker(c, "0", changes)
	defer workertest.DirtyKill(c, worker)

	close(changes)

	err := workertest.CheckKilled(c, worker)
	c.Check(err, tc.ErrorMatches, ".*watcher closed.*")
}

func (s *workerSuite) TestGetRequestErrorDoesNotKillWorker(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	got := make(chan struct{})
	s.facadeClient.EXPECT().GetSSHConnRequest(gomock.Any(), "tunnel-err").DoAndReturn(
		func(context.Context, string) (params.SSHConnRequestResult, error) {
			close(got)
			return params.SSHConnRequestResult{}, coreerrors.NotFound
		},
	)

	changes := make(chan []string)
	worker, _ := s.startWorker(c, "0", changes)
	defer workertest.DirtyKill(c, worker)

	select {
	case changes <- []string{"tunnel-err"}:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out sending change")
	}
	select {
	case <-got:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for request to be read")
	}

	// The worker must remain alive after a single request failure.
	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
}

var _ watcher.StringsWatcher = (*watchertest.MockStringsWatcher)(nil)
