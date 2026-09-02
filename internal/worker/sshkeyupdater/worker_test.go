// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshkeyupdater_test

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v4/ssh"
	sshtesting "github.com/juju/utils/v4/ssh/testing"
	"github.com/juju/worker/v5/workertest"
	gossh "golang.org/x/crypto/ssh"

	coremachineauthentication "github.com/juju/juju/core/machineauthentication"
	coressh "github.com/juju/juju/core/ssh"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/sshkeyupdater"
)

type workerSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.PatchValue(&sshkeyupdater.SSHUser, "")
}

func (*workerSuite) TestAddAndRemoveEphemeralKey(c *tc.C) {
	w, err := sshkeyupdater.NewWorker()
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	updater, ok := w.(coressh.EphemeralKeysUpdater)
	c.Assert(ok, tc.IsTrue)
	key, _, _, _, err := gossh.ParseAuthorizedKey([]byte(sshtesting.ValidKeyOne.Key))
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(updater.AddEphemeralKey(key, "tunnel-0"), tc.ErrorIsNil)
	keys, err := ssh.ListKeys(sshkeyupdater.SSHUser, ssh.FullKeys)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.DeepEquals, []string{sshtesting.ValidKeyOne.Key + " Juju:Ephemeral:tunnel-0"})

	c.Assert(updater.RemoveEphemeralKey(key), tc.ErrorIsNil)
	keys, err = ssh.ListKeys(sshkeyupdater.SSHUser, ssh.FullKeys)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.HasLen, 0)
}

// TestRemoveLastEphemeralKey ensures that removing the only authorized key
// succeeds and leaves the authorized_keys file empty.
func (*workerSuite) TestRemoveLastEphemeralKey(c *tc.C) {
	w, err := sshkeyupdater.NewWorker()
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	updater := w.(coressh.EphemeralKeysUpdater)
	key, _, _, _, err := gossh.ParseAuthorizedKey([]byte(sshtesting.ValidKeyOne.Key))
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(updater.AddEphemeralKey(key, "tunnel-0"), tc.ErrorIsNil)
	c.Assert(updater.RemoveEphemeralKey(key), tc.ErrorIsNil)

	keys, err := ssh.ListKeys(sshkeyupdater.SSHUser, ssh.FullKeys)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.HasLen, 0)
}

func (*workerSuite) TestEphemeralKeyOpsReturnWorkerDying(c *tc.C) {
	w, err := sshkeyupdater.NewWorker()
	c.Assert(err, tc.ErrorIsNil)
	updater := w.(coressh.EphemeralKeysUpdater)
	workertest.CleanKill(c, w)

	key, _, _, _, err := gossh.ParseAuthorizedKey([]byte(sshtesting.ValidKeyOne.Key))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(updater.AddEphemeralKey(key, "tunnel-0"), tc.ErrorIs, coremachineauthentication.ErrSShKeyUpdaterWorkerDying)
	c.Check(updater.RemoveEphemeralKey(key), tc.ErrorIs, coremachineauthentication.ErrSShKeyUpdaterWorkerDying)
}
