// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	stdtesting "testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/controller"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher/watchertest"
	controllersshservice "github.com/juju/juju/domain/ssh/service/controller"
)

type serviceSuite struct{}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestSSHServerHostKeyReturnsExisting(c *tc.C) {
	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().GetSSHServerHostKey(gomock.Any()).Return(testPrivateKey, nil)

	svc := controllersshservice.NewService(controllerState)
	key, err := svc.SSHServerHostKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *serviceSuite) TestSSHServerHostKeyErrorsWhenMissing(c *tc.C) {
	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().GetSSHServerHostKey(gomock.Any()).Return("", context.Canceled)

	svc := controllersshservice.NewService(controllerState)

	key, err := svc.SSHServerHostKey(c.Context())
	c.Check(key, tc.Equals, "")
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

// TestSSHServerHostPublicKeyReturnsStored checks the public key is returned
// directly from state without deriving it from the private key.
func (s *serviceSuite) TestSSHServerHostPublicKeyReturnsStored(c *tc.C) {
	signer, err := gossh.ParsePrivateKey([]byte(testPrivateKey))
	c.Assert(err, tc.ErrorIsNil)
	want := signer.PublicKey().Marshal()

	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().GetSSHServerHostPublicKey(gomock.Any()).Return(want, nil)
	svc := controllersshservice.NewService(controllerState)

	got, err := svc.SSHServerHostPublicKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, want)

}

// TestSSHServerHostPublicKeyErrorsWhenMissing checks that a state error
// fetching the public key is propagated to the caller.
func (s *serviceSuite) TestSSHServerHostPublicKeyErrorsWhenMissing(c *tc.C) {
	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().GetSSHServerHostPublicKey(gomock.Any()).Return(nil, context.Canceled)

	svc := controllersshservice.NewService(controllerState)

	got, err := svc.SSHServerHostPublicKey(c.Context())
	c.Check(got, tc.IsNil)
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *serviceSuite) TestGetPublicKeysForUser(c *tc.C) {
	keys := []coressh.PublicKey{{Key: "ssh-ed25519 AAAA"}}
	controllerState := NewMockState(gomock.NewController(c))
	username, err := user.NewName("alice")
	c.Assert(err, tc.ErrorIsNil)
	controllerState.EXPECT().GetPublicKeysForUser(gomock.Any(), username).Return(keys, nil)

	got, err := controllersshservice.NewService(controllerState).GetPublicKeysForUser(c.Context(), username)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, keys)
}

func (s *serviceSuite) TestPublicKeyInModel(c *tc.C) {
	signer, err := gossh.ParsePrivateKey([]byte(testPrivateKey))
	c.Assert(err, tc.ErrorIsNil)
	username, err := user.NewName("alice")
	c.Assert(err, tc.ErrorIsNil)
	modelUUID := coremodel.UUID("8419cd78-4993-4c3a-928e-c646226beeee")
	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().MatchesPublicKeyInModelForUser(gomock.Any(), modelUUID.String(), username.Name(), gossh.FingerprintSHA256(signer.PublicKey())).Return(true, nil)

	found, err := controllersshservice.NewService(controllerState).PublicKeyInModel(c.Context(), modelUUID, username, signer.PublicKey())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsTrue)
}

func (s *serviceSuite) TestPublicKeyInModelRejectsNonMatchingFingerprint(c *tc.C) {
	signer, err := gossh.ParsePrivateKey([]byte(testPrivateKey))
	c.Assert(err, tc.ErrorIsNil)
	username, err := user.NewName("alice")
	c.Assert(err, tc.ErrorIsNil)
	modelUUID := coremodel.UUID("8419cd78-4993-4c3a-928e-c646226beeee")
	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().MatchesPublicKeyInModelForUser(gomock.Any(), modelUUID.String(), username.Name(), gossh.FingerprintSHA256(signer.PublicKey())).Return(false, nil)

	found, err := controllersshservice.NewService(controllerState).PublicKeyInModel(c.Context(), modelUUID, username, signer.PublicKey())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsFalse)
}

func (s *serviceSuite) TestGetSSHServerPortReturnsStored(c *tc.C) {
	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().GetSSHServerPort(gomock.Any()).Return(17099, nil)

	port, err := controllersshservice.NewService(controllerState).GetSSHServerPort(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(port, tc.Equals, 17099)
}

func (s *serviceSuite) TestGetSSHServerPortDefaultsWhenMissing(c *tc.C) {
	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().GetSSHServerPort(gomock.Any()).Return(0, coreerrors.NotFound)

	port, err := controllersshservice.NewService(controllerState).GetSSHServerPort(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(port, tc.Equals, controller.DefaultSSHServerPort)
}

func (s *serviceSuite) TestGetSSHServerPortErrors(c *tc.C) {
	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().GetSSHServerPort(gomock.Any()).Return(0, context.Canceled)

	port, err := controllersshservice.NewService(controllerState).GetSSHServerPort(c.Context())
	c.Check(port, tc.Equals, 0)
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *serviceSuite) TestSetSSHServerPort(c *tc.C) {
	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().SetSSHServerPort(gomock.Any(), 17099).Return(nil)

	err := controllersshservice.NewService(controllerState).SetSSHServerPort(c.Context(), 17099)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetSSHServerPortErrors(c *tc.C) {
	controllerState := NewMockState(gomock.NewController(c))
	controllerState.EXPECT().SetSSHServerPort(gomock.Any(), 17099).Return(context.Canceled)

	err := controllersshservice.NewService(controllerState).SetSSHServerPort(c.Context(), 17099)
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *serviceSuite) TestWatchSSHServerPort(c *tc.C) {
	ctrl := gomock.NewController(c)
	controllerState := NewMockState(ctrl)
	watcherFactory := NewMockWatcherFactory(ctrl)

	controllerState.EXPECT().NamespaceForWatchSSHServerPort().Return("controller_ssh_server_port")

	w := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	watcherFactory.EXPECT().NewNotifyWatcher(gomock.Any(), gomock.Any(), gomock.Any()).Return(w, nil)

	svc := controllersshservice.NewWatchableService(controllerState, watcherFactory)
	got, err := svc.WatchSSHServerPort(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, w)
}

const testPrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\n" +
	"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz\n" +
	"c2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7VHoJY7LZ7yXzuWlSVYAAA\n" +
	"AIiZq0wRmatMEQAAAAtzc2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7V\n" +
	"HoJY7LZ7yXzuWlSVYAAAAEBYRsJTytYJUidtOuv3s3tdjyDA+4TSdCz9+hFKjyqz\n" +
	"v1PxSJ2ipSalQUUIYSFmEdYYTtUegljstnvJfO5aVJVgAAAAAAECAwQF\n" +
	"-----END OPENSSH PRIVATE KEY-----\n"
