// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki

import (
	"bytes"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

// DefaultLeaf is a default implementation of the Leaf interface
type DefaultLeaf struct {
	group          string
	certificate    *x509.Certificate
	chain          []*x509.Certificate
	signer         crypto.Signer
	tlsCertificate *tls.Certificate
}

// DefaultLeafRequest is a default implementation of the LeafRequest interface
type DefaultLeafRequest struct {
	dnsNames      set.Strings
	ipAddresses   map[string]net.IP
	leafMaker     LeafMaker
	requestSigner CertificateRequestSigner
	signer        crypto.Signer
	subject       pkix.Name
}

// Leaf represents a certificate and is associated key for signing operations.
type Leaf interface {
	// Certificate returns the x509 certificate of this leaf. May be nil if no
	// certificate exists yet. Call Commit to sign the leaf.
	Certificate() *x509.Certificate

	// Chain is the certificate signing chain for this leaf in the case of
	// intermediate CA's
	Chain() []*x509.Certificate

	// Signer is the crypto key used for signing operations on this leaf.
	Signer() crypto.Signer

	// Convenience method for generating a tls certificate for use in tls
	// transport.
	TLSCertificate() *tls.Certificate

	// Convenience method for converting this leaf to pem parts of
	// certificate/chain and private key
	ToPemParts() (cert, key []byte, err error)
}

// LeafMaker describes a function that can construct new Leaf's from the
// supplied certificate and crypto signer
type LeafMaker func(*x509.Certificate, []*x509.Certificate, crypto.Signer) (Leaf, error)

// LeafRequest is an intermediate unit for requesting new leafs with specific
// attributes.
type LeafRequest interface {
	// AddDNSNames adds the specificed dns names to the LeafRequest
	AddDNSNames(...string) LeafRequest

	// AddIPAddresses adds the specificed ip addresses to the LeafRequest
	AddIPAddresses(...net.IP) LeafRequest

	// Commit transforms the LeafRequest to a new Leaf
	Commit() (Leaf, error)
}

var (
	HeaderLeafGroup = "leaf.pki.juju.is/group"
)

// AddDNSNames implements LeafRequest AddDNSNames
func (d *DefaultLeafRequest) AddDNSNames(dnsNames ...string) LeafRequest {
	d.dnsNames = d.dnsNames.Union(set.NewStrings(dnsNames...))
	return d
}

// AddIPAddresses implements LeafRequest AddIPAddresses
func (d *DefaultLeafRequest) AddIPAddresses(ipAddresses ...net.IP) LeafRequest {
	for _, ipAddress := range ipAddresses {
		ipStr := ipAddress.String()
		if _, exists := d.ipAddresses[ipStr]; !exists {
			d.ipAddresses[ipStr] = ipAddress
		}
	}
	return d
}

// Certificate implements Leaf Certificate
func (d *DefaultLeaf) Certificate() *x509.Certificate {
	return d.certificate
}

// Chain implements Leaf Chain
func (d *DefaultLeaf) Chain() []*x509.Certificate {
	return d.chain
}

// Commit implements Leaf Commit
func (d *DefaultLeafRequest) Commit() (Leaf, error) {
	var err error

	if d.signer == nil {
		d.signer, err = DefaultKeyProfile()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	csr := &x509.CertificateRequest{
		DNSNames:    d.dnsNames.Values(),
		IPAddresses: ipAddressMapToSlice(d.ipAddresses),
		PublicKey:   d.signer.Public(),
		Subject:     d.subject,
	}

	cert, chain, err := d.requestSigner.SignCSR(csr)
	if err != nil {
		return nil, errors.Annotate(err, "signing CSR for leaf")
	}
	return d.leafMaker(cert, chain, d.signer)
}

// LeafHasDNSNames tests a diven Leaf to see if it contains the supplied DNS
// names
func LeafHasDNSNames(leaf Leaf, dnsNames []string) bool {
	certDNSNames := leaf.Certificate().DNSNames
	if len(certDNSNames) < len(dnsNames) {
		return false
	}

	a := make([]string, len(certDNSNames))
	copy(a, certDNSNames)
	sort.Strings(a)
	sort.Strings(dnsNames)

	for _, name := range dnsNames {
		index := sort.SearchStrings(a, name)
		if index == len(a) || a[index] != name {
			return false
		}
	}
	return true
}

func ipAddressMapToSlice(m map[string]net.IP) []net.IP {
	rval := make([]net.IP, len(m))
	i := 0
	for _, v := range m {
		rval[i] = v
		i = i + 1
	}
	return rval
}

// NewDefaultLeaf constructs a new DefaultLeaf for the supplied certificate and
// key
func NewDefaultLeaf(group string, cert *x509.Certificate,
	chain []*x509.Certificate, signer crypto.Signer) *DefaultLeaf {

	tlsCert := &tls.Certificate{
		Certificate: make([][]byte, len(chain)+1),
		PrivateKey:  signer,
		Leaf:        cert,
	}
	tlsCert.Certificate[0] = cert.Raw

	for i, chainCert := range chain {
		tlsCert.Certificate[i+1] = chainCert.Raw
	}

	return &DefaultLeaf{
		group:          group,
		certificate:    cert,
		chain:          chain,
		signer:         signer,
		tlsCertificate: tlsCert,
	}
}

// NewDefaultLeafPem constructs a new DefaultLeaf from the supplied PEM data
func NewDefaultLeafPem(group string, pemBlock []byte) (*DefaultLeaf, error) {
	certs, signers, err := UnmarshalPemData(pemBlock)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(certs) == 0 {
		return nil, errors.New("found zero certificates in pem bundle")
	}
	if len(signers) != 1 {
		return nil, errors.New("expected at least one private key in bundle")
	}
	if !PublicKeysEqual(signers[0].Public(), certs[0].PublicKey) {
		return nil, errors.New("public keys of first certificate and key do not match")
	}

	return NewDefaultLeaf(group, certs[0], certs[1:], signers[0]), nil
}

// NewDefaultLeafRequest create a DefaultLeafRequest object that implements
// LeafRequest
func NewDefaultLeafRequest(subject pkix.Name,
	requestSigner CertificateRequestSigner, maker LeafMaker) *DefaultLeafRequest {
	return &DefaultLeafRequest{
		dnsNames:      set.Strings{},
		ipAddresses:   map[string]net.IP{},
		leafMaker:     maker,
		requestSigner: requestSigner,
		subject:       subject,
	}
}

// NewDefaultLeafRequestWithSigner create a DefaultLeafRequest object that
// implements LeafRequest. Takes a default signer to use for all certificate
// creation instead of generating a new one.
func NewDefaultLeafRequestWithSigner(subject pkix.Name, signer crypto.Signer,
	requestSigner CertificateRequestSigner,
	maker LeafMaker) *DefaultLeafRequest {
	return &DefaultLeafRequest{
		dnsNames:      set.Strings{},
		ipAddresses:   map[string]net.IP{},
		leafMaker:     maker,
		requestSigner: requestSigner,
		signer:        signer,
		subject:       subject,
	}
}

// Signer implements Leaf interface Signer
func (d *DefaultLeaf) Signer() crypto.Signer {
	return d.signer
}

// TLSCertificate implements Leaf interface TLSCertificate
func (d *DefaultLeaf) TLSCertificate() *tls.Certificate {
	return d.tlsCertificate
}

// ToPemParts implements Leaf interface ToPemParts
func (d *DefaultLeaf) ToPemParts() ([]byte, []byte, error) {
	certBuf := bytes.Buffer{}
	err := CertificateToPemWriter(&certBuf, map[string]string{
		HeaderLeafGroup: d.group,
	}, d.Certificate(), d.Chain()...)
	if err != nil {
		return nil, nil, errors.Annotate(err, "turning leaf certificate to pem")
	}

	keyBuf := bytes.Buffer{}
	err = SignerToPemWriter(&keyBuf, d.Signer())
	if err != nil {
		return nil, nil, errors.Annotate(err, "turning leaf key to pem")
	}

	return certBuf.Bytes(), keyBuf.Bytes(), nil
}
