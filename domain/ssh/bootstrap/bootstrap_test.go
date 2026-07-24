// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	stdtesting "testing"

	"github.com/google/uuid"
	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	schematesting "github.com/juju/juju/domain/schema/testing"
	domainssh "github.com/juju/juju/domain/ssh"
	jujutesting "github.com/juju/juju/internal/testing"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

func TestBootstrapSuite(t *stdtesting.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) TestInsertInitialSSHServerHostKey(c *tc.C) {
	err := InsertInitialSSHServerHostKey(jujutesting.SSHServerHostKey)(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	var (
		algorithmTypeID int
		key             string
		publicKey       []byte
	)
	row := s.DB().QueryRow(`SELECT algorithm_type_id, ssh_key, public_key FROM controller_ssh_host_key WHERE id = ?`, domainssh.SSHServerHostKeyUUID)
	c.Assert(row.Scan(&algorithmTypeID, &key, &publicKey), tc.ErrorIsNil)
	c.Check(algorithmTypeID, tc.Equals, domainssh.SSHKeyAlgorithmTypeED25519ID)
	c.Check(key, tc.Equals, jujutesting.SSHServerHostKey)

	// The stored public key must match the one derived from the private key.
	signer, err := gossh.ParsePrivateKey([]byte(jujutesting.SSHServerHostKey))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(publicKey, tc.DeepEquals, signer.PublicKey().Marshal())
}

func (s *bootstrapSuite) TestSSHServerHostKeyUUID(c *tc.C) {
	namespaceUUID, err := uuid.Parse(domainssh.WellKnownUUIDNamespace)
	c.Assert(err, tc.ErrorIsNil)

	wellKnownUUID := uuid.NewSHA1(namespaceUUID, []byte(domainssh.SSHServerHostKeyWellKnownName))
	c.Check(wellKnownUUID.String(), tc.Equals, domainssh.SSHServerHostKeyUUID)
}

func (s *bootstrapSuite) TestInsertInitialSSHServerHostKeyValidatesEmpty(c *tc.C) {
	err := InsertInitialSSHServerHostKey("")(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorMatches, `empty SSHServerHostKey`)
}
