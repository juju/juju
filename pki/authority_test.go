// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki_test

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"net"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/pki"
)

type AuthoritySuite struct {
	ca     *x509.Certificate
	signer crypto.Signer
}

var _ = gc.Suite(&AuthoritySuite{})

func (a *AuthoritySuite) SetUpTest(c *gc.C) {
	signer, err := pki.DefaultKeyProfile()
	c.Assert(err, jc.ErrorIsNil)
	a.signer = signer

	commonName := "juju-test-ca"
	ca, err := pki.NewCA(commonName, a.signer)
	c.Assert(err, jc.ErrorIsNil)

	a.ca = ca
	c.Assert(a.ca.Subject.CommonName, gc.Equals, commonName)
	c.Assert(a.ca.Subject.Organization, jc.DeepEquals, pki.Organisation)
	c.Assert(a.ca.BasicConstraintsValid, gc.Equals, true)
	c.Assert(a.ca.IsCA, gc.Equals, true)
}

func (a *AuthoritySuite) TestNewAuthority(c *gc.C) {
	authority, err := pki.NewDefaultAuthority(a.ca, a.signer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(authority.Certificate(), jc.DeepEquals, a.ca)
	c.Assert(authority.Signer(), jc.DeepEquals, a.signer)
	c.Assert(len(authority.Chain()), gc.Equals, 0)
}

func (a *AuthoritySuite) TestMissingLeafGroup(c *gc.C) {
	authority, err := pki.NewDefaultAuthority(a.ca, a.signer)
	c.Assert(err, jc.ErrorIsNil)
	leaf, err := authority.LeafForGroup("noexist")
	c.Assert(err, gc.NotNil)
	c.Assert(leaf, gc.IsNil)
	c.Assert(errors.IsNotFound(err), gc.Equals, true)
}

func (a *AuthoritySuite) TestLeafRequest(c *gc.C) {
	authority, err := pki.NewDefaultAuthority(a.ca, a.signer)
	c.Assert(err, jc.ErrorIsNil)
	dnsNames := []string{"test.juju.is"}
	ipAddresses := []net.IP{net.ParseIP("fe80:abcd::1")}
	leaf, err := authority.LeafRequestForGroup("testgroup").
		AddDNSNames(dnsNames...).
		AddIPAddresses(ipAddresses...).
		Commit()

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leaf.Certificate().DNSNames, jc.DeepEquals, dnsNames)
	c.Assert(leaf.Certificate().IPAddresses, jc.DeepEquals, ipAddresses)

	leaf, err = authority.LeafForGroup("testgroup")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leaf.Certificate().DNSNames, jc.DeepEquals, dnsNames)
	c.Assert(leaf.Certificate().IPAddresses, jc.DeepEquals, ipAddresses)
}

func (a *AuthoritySuite) TestLeafRequestChain(c *gc.C) {
	authority, err := pki.NewDefaultAuthority(a.ca, a.signer)
	c.Assert(err, jc.ErrorIsNil)
	dnsNames := []string{"test.juju.is"}
	ipAddresses := []net.IP{net.ParseIP("fe80:abcd::1")}
	leaf, err := authority.LeafRequestForGroup("testgroup").
		AddDNSNames(dnsNames...).
		AddIPAddresses(ipAddresses...).
		Commit()

	chain := leaf.Chain()
	c.Assert(len(chain), gc.Equals, 1)
	c.Assert(chain[0], jc.DeepEquals, authority.Certificate())
}

func (a *AuthoritySuite) TestLeafFromPem(c *gc.C) {
	authority, err := pki.NewDefaultAuthority(a.ca, a.signer)
	c.Assert(err, jc.ErrorIsNil)
	dnsNames := []string{"test.juju.is"}
	ipAddresses := []net.IP{net.ParseIP("fe80:abcd::1")}
	leaf, err := authority.LeafRequestForGroup("testgroup").
		AddDNSNames(dnsNames...).
		AddIPAddresses(ipAddresses...).
		Commit()
	c.Assert(err, jc.ErrorIsNil)

	cert, key, err := leaf.ToPemParts()
	c.Assert(err, jc.ErrorIsNil)

	authority1, err := pki.NewDefaultAuthority(a.ca, a.signer)
	c.Assert(err, jc.ErrorIsNil)

	leaf1, err := authority1.LeafGroupFromPemCertKey("testgroup", cert, key)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(leaf1, jc.DeepEquals, leaf)

	leaf2, err := authority.LeafForGroup("testgroup")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leaf2, gc.NotNil)
}

func (a *AuthoritySuite) TestAuthorityFromPemBlock(c *gc.C) {
	caBytes := bytes.Buffer{}
	err := pki.CertificateToPemWriter(&caBytes, map[string]string{}, a.ca)
	c.Assert(err, jc.ErrorIsNil)

	keyBytes := bytes.Buffer{}
	err = pki.SignerToPemWriter(&keyBytes, a.signer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = pki.NewDefaultAuthorityPem(append(caBytes.Bytes(), keyBytes.Bytes()...))
	c.Assert(err, jc.ErrorIsNil)
}

func (a *AuthoritySuite) TestAuthorityFromPemCAKey(c *gc.C) {
	caBytes := bytes.Buffer{}
	err := pki.CertificateToPemWriter(&caBytes, map[string]string{}, a.ca)
	c.Assert(err, jc.ErrorIsNil)

	keyBytes := bytes.Buffer{}
	err = pki.SignerToPemWriter(&keyBytes, a.signer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = pki.NewDefaultAuthorityPemCAKey(caBytes.Bytes(), keyBytes.Bytes())
	c.Assert(err, jc.ErrorIsNil)
}
