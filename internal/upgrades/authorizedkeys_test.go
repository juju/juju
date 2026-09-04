// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/ssh"
	sshtesting "github.com/juju/utils/v4/ssh/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
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

// iaasContext returns an upgrade context suitable for an IAAS machine.
func iaasContext() Context {
	return NewContext(&stubAgentConfig{values: map[string]string{}}, nil)
}

// caasContext returns an upgrade context suitable for a CAAS machine.
func caasContext() Context {
	return NewContext(&stubAgentConfig{values: map[string]string{
		agent.ProviderType: constants.CAASProviderType,
	}}, nil)
}

// stubAgentConfig is a minimal agent config that reports the configured
// values, enough for the authorized keys upgrade step.
type stubAgentConfig struct {
	agent.ConfigSetter
	values map[string]string
}

func (s *stubAgentConfig) Value(name string) string {
	return s.values[name]
}

func (*authorizedKeysSuite) TestRemovePersistentJujuAuthorizedKeys(c *tc.C) {
	persistentKey := sshtesting.ValidKeyOne.Key + " Juju:user@host"
	ephemeralKey := sshtesting.ValidKeyTwo.Key + " Juju:Ephemeral:tunnel-0"
	manualKey := sshtesting.ValidKeyThree.Key + " manual@example.com"
	c.Assert(ssh.AddKeys(sshUser, persistentKey, ephemeralKey, manualKey), tc.ErrorIsNil)

	c.Assert(removePersistentJujuAuthorizedKeys(iaasContext()), tc.ErrorIsNil)

	keys, err := ssh.ListKeys(sshUser, ssh.FullKeys)
	c.Assert(err, tc.ErrorIsNil)
	expected := []string{ephemeralKey, manualKey}
	slices.Sort(keys)
	slices.Sort(expected)
	c.Check(keys, tc.DeepEquals, expected)

	// The step is idempotent: the persistent key is already gone.
	c.Assert(removePersistentJujuAuthorizedKeys(iaasContext()), tc.ErrorIsNil)
}

func (*authorizedKeysSuite) TestRemovePersistentJujuAuthorizedKeysEmpty(c *tc.C) {
	c.Assert(removePersistentJujuAuthorizedKeys(iaasContext()), tc.ErrorIsNil)
}

func (*authorizedKeysSuite) TestRemovePersistentJujuAuthorizedKeysAllEphemeral(c *tc.C) {
	ephemeralKey := sshtesting.ValidKeyOne.Key + " Juju:Ephemeral:tunnel-0"
	c.Assert(ssh.AddKeys(sshUser, ephemeralKey), tc.ErrorIsNil)

	c.Assert(removePersistentJujuAuthorizedKeys(iaasContext()), tc.ErrorIsNil)

	keys, err := ssh.ListKeys(sshUser, ssh.FullKeys)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.DeepEquals, []string{ephemeralKey})
}

func (*authorizedKeysSuite) TestRemovePersistentJujuAuthorizedKeysMalformed(c *tc.C) {
	key := sshtesting.ValidKeyOne.Key + " Juju:user@host"
	c.Assert(ssh.AddKeys(sshUser, key), tc.ErrorIsNil)

	file := filepath.Join(utils.Home(), ".ssh", authorizedKeysFile)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
	c.Assert(err, tc.ErrorIsNil)
	_, err = f.WriteString("not an ssh key\n")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.Close(), tc.ErrorIsNil)

	c.Assert(removePersistentJujuAuthorizedKeys(iaasContext()), tc.ErrorIsNil)
	keys, err := ssh.ListKeys(sshUser, ssh.FullKeys)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.HasLen, 0)
}

// TestRemovePersistentJujuAuthorizedKeysCAAS ensures the step is skipped
// entirely on CAAS machines, which have no cloud-init injected keys.
func (*authorizedKeysSuite) TestRemovePersistentJujuAuthorizedKeysCAAS(c *tc.C) {
	persistentKey := sshtesting.ValidKeyOne.Key + " Juju:user@host"
	c.Assert(ssh.AddKeys(sshUser, persistentKey), tc.ErrorIsNil)

	c.Assert(removePersistentJujuAuthorizedKeys(caasContext()), tc.ErrorIsNil)

	keys, err := ssh.ListKeys(sshUser, ssh.FullKeys)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.DeepEquals, []string{persistentKey})
}
