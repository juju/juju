// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/oci/common"
	ocitesting "github.com/juju/juju/internal/provider/oci/testing"
	"github.com/juju/juju/internal/testing"
)

type clientSuite struct {
	testing.BaseSuite

	config *common.JujuConfigProvider
}

func TestClientSuite(t *stdtesting.T) {
	tc.Run(t, &clientSuite{})
}

func (s *clientSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *clientSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.config = &common.JujuConfigProvider{
		Key:         []byte(ocitesting.PrivateKeyUnencrypted),
		Fingerprint: ocitesting.PrivateKeyUnencryptedFingerprint,
		Tenancy:     "fake",
		User:        "fake",
		OCIRegion:   "us-phoenix-1",
	}
}

func (s *clientSuite) TestValidateKeyUnencrypted(c *tc.C) {
	err := common.ValidateKey([]byte(ocitesting.PrivateKeyUnencrypted), "")
	c.Assert(err, tc.IsNil)

	err = common.ValidateKey([]byte(ocitesting.PrivateKeyUnencrypted), "so secret")
	c.Assert(err, tc.IsNil)

	err = common.ValidateKey([]byte("bogus"), "")
	c.Check(err, tc.ErrorMatches, "invalid private key")
}

func (s *clientSuite) TestValidateKeyEncrypted(c *tc.C) {
	err := common.ValidateKey([]byte(ocitesting.PrivateKeyEncrypted), ocitesting.PrivateKeyPassphrase)
	c.Assert(err, tc.IsNil)

	err = common.ValidateKey([]byte(ocitesting.PrivateKeyEncrypted), "wrong passphrase")
	c.Assert(err, tc.ErrorMatches, "decrypting private key: x509: decryption password incorrect")

	// empty passphrase
	err = common.ValidateKey([]byte(ocitesting.PrivateKeyEncrypted), "")
	c.Assert(err, tc.ErrorMatches, "decrypting private key: x509: decryption password incorrect")
}

func (s *clientSuite) TestTenancyOCID(c *tc.C) {
	ocid, err := s.config.TenancyOCID()
	c.Assert(err, tc.IsNil)
	c.Check(ocid, tc.Equals, "fake")

	s.config.Tenancy = ""
	ocid, err = s.config.TenancyOCID()
	c.Assert(err, tc.ErrorMatches, "tenancyOCID is not set")
	c.Check(ocid, tc.Equals, "")
}

func (s *clientSuite) TestUserOCID(c *tc.C) {
	ocid, err := s.config.UserOCID()
	c.Assert(err, tc.IsNil)
	c.Check(ocid, tc.Equals, "fake")

	s.config.User = ""
	ocid, err = s.config.UserOCID()
	c.Assert(err, tc.ErrorMatches, "userOCID is not set")
	c.Check(ocid, tc.Equals, "")
}

func (s *clientSuite) TestKeyFingerprint(c *tc.C) {
	fp, err := s.config.KeyFingerprint()
	c.Assert(err, tc.IsNil)
	c.Check(fp, tc.Equals, ocitesting.PrivateKeyUnencryptedFingerprint)

	s.config.Fingerprint = ""
	fp, err = s.config.KeyFingerprint()
	c.Assert(err, tc.ErrorMatches, "Fingerprint is not set")
	c.Check(fp, tc.Equals, "")
}

func (s *clientSuite) TestRegion(c *tc.C) {
	region, err := s.config.Region()
	c.Assert(err, tc.IsNil)
	c.Check(region, tc.Equals, "us-phoenix-1")

	s.config.OCIRegion = ""
	region, err = s.config.Region()
	c.Assert(err, tc.ErrorMatches, "Region is not set")
	c.Check(region, tc.Equals, "")
}

func (s *clientSuite) TestPrivateRSAKey(c *tc.C) {
	pkey, err := s.config.PrivateRSAKey()
	c.Assert(err, tc.IsNil)
	c.Assert(pkey, tc.NotNil)

	s.config.Key = []byte(ocitesting.PrivateKeyEncrypted)
	pkey, err = s.config.PrivateRSAKey()
	c.Assert(err, tc.ErrorMatches, "x509: decryption password incorrect")
	c.Assert(pkey, tc.IsNil)

	s.config.Passphrase = ocitesting.PrivateKeyPassphrase
	_, err = s.config.PrivateRSAKey()
	c.Assert(err, tc.IsNil)
}

func (s *clientSuite) TestKeyID(c *tc.C) {
	id := fmt.Sprintf("%s/%s/%s", s.config.Tenancy, s.config.User, s.config.Fingerprint)
	keyID, err := s.config.KeyID()
	c.Assert(err, tc.IsNil)
	c.Check(keyID, tc.Equals, id)

	s.config.Tenancy = ""
	_, err = s.config.KeyID()
	c.Assert(err, tc.ErrorMatches, "config provider is not properly initialized")
}
