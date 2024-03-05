// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/provider/oci/common"
	ocitesting "github.com/juju/juju/internal/provider/oci/testing"
	"github.com/juju/juju/testing"
)

type clientSuite struct {
	testing.BaseSuite

	config *common.JujuConfigProvider
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.config = &common.JujuConfigProvider{
		Key:         []byte(ocitesting.PrivateKeyUnencrypted),
		Fingerprint: ocitesting.PrivateKeyUnencryptedFingerprint,
		Tenancy:     "fake",
		User:        "fake",
		OCIRegion:   "us-phoenix-1",
	}
}

func (s *clientSuite) TestValidateKeyUnencrypted(c *gc.C) {
	err := common.ValidateKey([]byte(ocitesting.PrivateKeyUnencrypted), "")
	c.Assert(err, gc.IsNil)

	err = common.ValidateKey([]byte(ocitesting.PrivateKeyUnencrypted), "so secret")
	c.Assert(err, gc.IsNil)

	err = common.ValidateKey([]byte("bogus"), "")
	c.Check(err, gc.ErrorMatches, "invalid private key")
}

func (s *clientSuite) TestValidateKeyEncrypted(c *gc.C) {
	err := common.ValidateKey([]byte(ocitesting.PrivateKeyEncrypted), ocitesting.PrivateKeyPassphrase)
	c.Assert(err, gc.IsNil)

	err = common.ValidateKey([]byte(ocitesting.PrivateKeyEncrypted), "wrong passphrase")
	c.Assert(err, gc.ErrorMatches, "decrypting private key: x509: decryption password incorrect")

	// empty passphrase
	err = common.ValidateKey([]byte(ocitesting.PrivateKeyEncrypted), "")
	c.Assert(err, gc.ErrorMatches, "decrypting private key: x509: decryption password incorrect")
}

func (s *clientSuite) TestTenancyOCID(c *gc.C) {
	ocid, err := s.config.TenancyOCID()
	c.Assert(err, gc.IsNil)
	c.Check(ocid, gc.Equals, "fake")

	s.config.Tenancy = ""
	ocid, err = s.config.TenancyOCID()
	c.Assert(err, gc.ErrorMatches, "tenancyOCID is not set")
	c.Check(ocid, gc.Equals, "")
}

func (s *clientSuite) TestUserOCID(c *gc.C) {
	ocid, err := s.config.UserOCID()
	c.Assert(err, gc.IsNil)
	c.Check(ocid, gc.Equals, "fake")

	s.config.User = ""
	ocid, err = s.config.UserOCID()
	c.Assert(err, gc.ErrorMatches, "userOCID is not set")
	c.Check(ocid, gc.Equals, "")
}

func (s *clientSuite) TestKeyFingerprint(c *gc.C) {
	fp, err := s.config.KeyFingerprint()
	c.Assert(err, gc.IsNil)
	c.Check(fp, gc.Equals, ocitesting.PrivateKeyUnencryptedFingerprint)

	s.config.Fingerprint = ""
	fp, err = s.config.KeyFingerprint()
	c.Assert(err, gc.ErrorMatches, "Fingerprint is not set")
	c.Check(fp, gc.Equals, "")
}

func (s *clientSuite) TestRegion(c *gc.C) {
	region, err := s.config.Region()
	c.Assert(err, gc.IsNil)
	c.Check(region, gc.Equals, "us-phoenix-1")

	s.config.OCIRegion = ""
	region, err = s.config.Region()
	c.Assert(err, gc.ErrorMatches, "Region is not set")
	c.Check(region, gc.Equals, "")
}

func (s *clientSuite) TestPrivateRSAKey(c *gc.C) {
	pkey, err := s.config.PrivateRSAKey()
	c.Assert(err, gc.IsNil)
	c.Assert(pkey, gc.NotNil)

	s.config.Key = []byte(ocitesting.PrivateKeyEncrypted)
	pkey, err = s.config.PrivateRSAKey()
	c.Assert(err, gc.ErrorMatches, "x509: decryption password incorrect")
	c.Assert(pkey, gc.IsNil)

	s.config.Passphrase = ocitesting.PrivateKeyPassphrase
	_, err = s.config.PrivateRSAKey()
	c.Assert(err, gc.IsNil)
}

func (s *clientSuite) TestKeyID(c *gc.C) {
	id := fmt.Sprintf("%s/%s/%s", s.config.Tenancy, s.config.User, s.config.Fingerprint)
	keyID, err := s.config.KeyID()
	c.Assert(err, gc.IsNil)
	c.Check(keyID, gc.Equals, id)

	s.config.Tenancy = ""
	_, err = s.config.KeyID()
	c.Assert(err, gc.ErrorMatches, "config provider is not properly initialized")
}
