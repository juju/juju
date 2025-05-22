// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki_test

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"net"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/pki"
)

type AuthoritySuite struct {
	ca     *x509.Certificate
	signer crypto.Signer
}

func TestAuthoritySuite(t *testing.T) {
	tc.Run(t, &AuthoritySuite{})
}

func (a *AuthoritySuite) SetUpTest(c *tc.C) {
	signer, err := pki.DefaultKeyProfile()
	c.Assert(err, tc.ErrorIsNil)
	a.signer = signer

	commonName := "juju-test-ca"
	ca, err := pki.NewCA(commonName, a.signer)
	c.Assert(err, tc.ErrorIsNil)

	a.ca = ca
	c.Assert(a.ca.Subject.CommonName, tc.Equals, commonName)
	c.Assert(a.ca.Subject.Organization, tc.DeepEquals, pki.Organisation)
	c.Assert(a.ca.BasicConstraintsValid, tc.Equals, true)
	c.Assert(a.ca.IsCA, tc.Equals, true)
}

func (a *AuthoritySuite) TestNewAuthority(c *tc.C) {
	authority, err := pki.NewDefaultAuthority(a.ca, a.signer)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(authority.Certificate(), tc.DeepEquals, a.ca)
	mc := tc.NewMultiChecker()
	// Ignore the computed speedups for RSA since comparing byte for byte
	// of bignumbers may fail due to superfluous zeros (despite the numbers matching)
	mc.AddExpr("_.Precomputed", tc.Ignore)
	c.Assert(authority.Signer(), mc, a.signer)
	c.Assert(len(authority.Chain()), tc.Equals, 0)
}

func (a *AuthoritySuite) TestMissingLeafGroup(c *tc.C) {
	authority, err := pki.NewDefaultAuthority(a.ca, a.signer)
	c.Assert(err, tc.ErrorIsNil)
	leaf, err := authority.LeafForGroup("noexist")
	c.Assert(err, tc.NotNil)
	c.Assert(leaf, tc.IsNil)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (a *AuthoritySuite) TestLeafRequest(c *tc.C) {
	authority, err := pki.NewDefaultAuthority(a.ca, a.signer)
	c.Assert(err, tc.ErrorIsNil)
	dnsNames := []string{"test.juju.is"}
	ipAddresses := []net.IP{net.ParseIP("fe80:abcd::1")}
	leaf, err := authority.LeafRequestForGroup("testgroup").
		AddDNSNames(dnsNames...).
		AddIPAddresses(ipAddresses...).
		Commit()

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leaf.Certificate().DNSNames, tc.DeepEquals, dnsNames)
	c.Assert(leaf.Certificate().IPAddresses, tc.DeepEquals, ipAddresses)

	leaf, err = authority.LeafForGroup("testgroup")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leaf.Certificate().DNSNames, tc.DeepEquals, dnsNames)
	c.Assert(leaf.Certificate().IPAddresses, tc.DeepEquals, ipAddresses)
}

func (a *AuthoritySuite) TestLeafRequestChain(c *tc.C) {
	authority, err := pki.NewDefaultAuthority(a.ca, a.signer)
	c.Assert(err, tc.ErrorIsNil)
	dnsNames := []string{"test.juju.is"}
	ipAddresses := []net.IP{net.ParseIP("fe80:abcd::1")}
	leaf, err := authority.LeafRequestForGroup("testgroup").
		AddDNSNames(dnsNames...).
		AddIPAddresses(ipAddresses...).
		Commit()
	c.Assert(err, tc.ErrorIsNil)
	chain := leaf.Chain()
	c.Assert(len(chain), tc.Equals, 1)
	c.Assert(chain[0], tc.DeepEquals, authority.Certificate())
}

func (a *AuthoritySuite) TestLeafFromPem(c *tc.C) {
	authority, err := pki.NewDefaultAuthority(a.ca, a.signer)
	c.Assert(err, tc.ErrorIsNil)
	dnsNames := []string{"test.juju.is"}
	ipAddresses := []net.IP{net.ParseIP("fe80:abcd::1")}
	leaf, err := authority.LeafRequestForGroup("testgroup").
		AddDNSNames(dnsNames...).
		AddIPAddresses(ipAddresses...).
		Commit()
	c.Assert(err, tc.ErrorIsNil)

	cert, key, err := leaf.ToPemParts()
	c.Assert(err, tc.ErrorIsNil)

	authority1, err := pki.NewDefaultAuthority(a.ca, a.signer)
	c.Assert(err, tc.ErrorIsNil)

	leaf1, err := authority1.LeafGroupFromPemCertKey("testgroup", cert, key)
	c.Assert(err, tc.ErrorIsNil)

	mc := tc.NewMultiChecker()
	// Ignore the computed speedups for RSA since comparing byte for byte
	// of bignumbers may fail due to superfluous zeros (despite the numbers matching)
	mc.AddExpr("_.signer.Precomputed", tc.Ignore)
	c.Assert(leaf1, mc, leaf)

	leaf2, err := authority.LeafForGroup("testgroup")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leaf2, tc.NotNil)
}

func (a *AuthoritySuite) TestAuthorityFromPemBlock(c *tc.C) {
	caBytes := bytes.Buffer{}
	err := pki.CertificateToPemWriter(&caBytes, map[string]string{}, a.ca)
	c.Assert(err, tc.ErrorIsNil)

	keyBytes := bytes.Buffer{}
	err = pki.SignerToPemWriter(&keyBytes, a.signer)
	c.Assert(err, tc.ErrorIsNil)

	_, err = pki.NewDefaultAuthorityPem(append(caBytes.Bytes(), keyBytes.Bytes()...))
	c.Assert(err, tc.ErrorIsNil)
}

func (a *AuthoritySuite) TestAuthorityFromPemCAKey(c *tc.C) {
	caBytes := bytes.Buffer{}
	err := pki.CertificateToPemWriter(&caBytes, map[string]string{}, a.ca)
	c.Assert(err, tc.ErrorIsNil)

	keyBytes := bytes.Buffer{}
	err = pki.SignerToPemWriter(&keyBytes, a.signer)
	c.Assert(err, tc.ErrorIsNil)

	_, err = pki.NewDefaultAuthorityPemCAKey(caBytes.Bytes(), keyBytes.Bytes())
	c.Assert(err, tc.ErrorIsNil)
}
