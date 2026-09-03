// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"slices"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v4/ssh"
	sshtesting "github.com/juju/utils/v4/ssh/testing"

	coretesting "github.com/juju/juju/internal/testing"
)

type authorizedKeysSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

func TestAuthorizedKeysSuite(t *testing.T) {
	tc.Run(t, &authorizedKeysSuite{})
}

func (s *authorizedKeysSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.PatchValue(&sshUser, "")
}

func (*authorizedKeysSuite) TestRemovePersistentJujuAuthorizedKeys(c *tc.C) {
	persistentKey := sshtesting.ValidKeyOne.Key + " Juju:user@host"
	ephemeralKey := sshtesting.ValidKeyTwo.Key + " Juju:Ephemeral:tunnel-0"
	manualKey := sshtesting.ValidKeyThree.Key + " manual@example.com"
	c.Assert(ssh.AddKeys(sshUser, persistentKey, ephemeralKey, manualKey), tc.ErrorIsNil)

	c.Assert(removePersistentJujuAuthorizedKeys(nil), tc.ErrorIsNil)

	keys, err := ssh.ListKeys(sshUser, ssh.FullKeys)
	c.Assert(err, tc.ErrorIsNil)
	expected := []string{ephemeralKey, manualKey}
	slices.Sort(keys)
	slices.Sort(expected)
	c.Check(keys, tc.DeepEquals, expected)
}
