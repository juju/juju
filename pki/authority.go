// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki

import (
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
)

const (
	DefaultLeafGroup      = "controller"
	ControllerIPLeafGroup = "controllerip"
)

// Authority represents a secure means of issuing groups of common interest
// certificates that share a certificate authority. Authority should
// only be shared around between trusted parties. Authority should be considered
// thread safe.
type Authority interface {
	// Leaf Authority implements the Leaf interface
	Leaf

	// LeafForGroup returns the leaf associated with the given group. Returns
	// error if no leaf exists for the given group.
	LeafForGroup(string) (Leaf, error)

	// LeafGroupFromPemCertKey loads an already existing certificate key pair as
	// a new leaf at the given group. Returns error if a leaf for the given
	// group already exists or an error occurred loading the pem data.
	LeafGroupFromPemCertKey(group string, certPem, key []byte) (Leaf, error)

	// LeafRequestForGroup starts a new leaf request for the given group. If a
	// leaf already exists it will be overwritten with this request when
	// committed.
	LeafRequestForGroup(string) LeafRequest

	// LeafRange is a method for safely iterating over all the leafs for the
	// given Authority. Supplied function should return false to stop iteration
	// early.
	LeafRange(func(leaf Leaf) bool)
}

// DefaultAuthority is a juju implementation of the Authority interface. It's
// main difference is the ability to set a common leaf private key so all leafs
// use the same key.
type DefaultAuthority struct {
	authority       Leaf
	leafs           sync.Map
	leafSignerMutex sync.Mutex
	leafSigner      crypto.Signer
}

// Organisation default organisation set on all certificates
var Organisation = []string{"Juju"}

// LeafSubjectTemplate is the default pkix.Name used for all leaf certificates
// made from a DefaultAuthority
var LeafSubjectTemplate = pkix.Name{
	Organization: Organisation,
	CommonName:   "Juju server certificate",
}

// Certificate implements Leaf interface method. Returns the CA's certificate
func (a *DefaultAuthority) Certificate() *x509.Certificate {
	return a.authority.Certificate()
}

// Chain implements Leaf interface method. Returns the CA's chain if it is an
// intermediate.
func (a *DefaultAuthority) Chain() []*x509.Certificate {
	return a.authority.Chain()
}

func (a *DefaultAuthority) ChainWithAuthority() []*x509.Certificate {
	chain := a.authority.Chain()
	if chain == nil {
		chain = []*x509.Certificate{}
	}
	return append(chain, a.authority.Certificate())
}

// leafMaker is responsible for providing a method to make new leafs after
// request signing.
func (a *DefaultAuthority) leafMaker(groupKey string) LeafMaker {
	return func(cert *x509.Certificate, chain []*x509.Certificate,
		signer crypto.Signer) (Leaf, error) {
		leaf := NewDefaultLeaf(groupKey, cert, chain, signer)
		a.leafs.Store(groupKey, leaf)
		return leaf, nil
	}
}

// LeafRequestForGroup implements Authority interface method. Starts a new leaf
// request for the given group overwritting any existing leaf when the request
// is committed.
func (a *DefaultAuthority) LeafRequestForGroup(group string) LeafRequest {
	groupKey := strings.ToLower(group)
	subject := MakeX509NameFromDefaults(&LeafSubjectTemplate,
		&pkix.Name{
			CommonName: fmt.Sprintf("%s - %s", LeafSubjectTemplate.CommonName, groupKey),
		})
	a.leafSignerMutex.Lock()
	defer a.leafSignerMutex.Unlock()
	if a.leafSigner != nil {
		return NewDefaultLeafRequestWithSigner(subject, a.leafSigner,
			NewDefaultRequestSigner(a.Certificate(), a.ChainWithAuthority(), a.Signer()),
			a.leafMaker(groupKey))
	}
	return NewDefaultLeafRequest(subject,
		NewDefaultRequestSigner(a.Certificate(), a.ChainWithAuthority(), a.Signer()),
		a.leafMaker(groupKey))
}

// LeafForGroup implements Authority interface method.
func (a *DefaultAuthority) LeafForGroup(group string) (Leaf, error) {
	groupKey := strings.ToLower(group)
	leaf, has := a.leafs.Load(groupKey)
	if !has {
		return nil, errors.NotFoundf("no leaf for group key %s", groupKey)
	}
	return leaf.(Leaf), nil
}

// LeafGroupFromPemCertKey implements Authority interface method.
func (a *DefaultAuthority) LeafGroupFromPemCertKey(group string,
	certPem, key []byte) (Leaf, error) {

	groupKey := strings.ToLower(group)
	certs, signers, err := UnmarshalPemData(append(certPem, key...))
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

	leaf := NewDefaultLeaf(groupKey, certs[0], certs[1:], signers[0])

	if _, exists := a.leafs.LoadOrStore(groupKey, leaf); exists {
		return nil, errors.AlreadyExistsf("leaf for group %s", group)
	}
	return leaf, nil
}

// LeafRange implements Authority interface method.
func (a *DefaultAuthority) LeafRange(ranger func(leaf Leaf) bool) {
	a.leafs.Range(func(_, val interface{}) bool {
		return ranger(val.(Leaf))
	})
}

// Helper method to generate a new certificate authority using the provided
// common name and signer.
func NewCA(commonName string, signer crypto.Signer) (*x509.Certificate, error) {
	template := &x509.Certificate{}
	if err := assetTagCertificate(template); err != nil {
		return nil, errors.Annotate(err, "failed tagging new CA certificate")
	}

	template.Subject = pkix.Name{
		CommonName:   commonName,
		Organization: Organisation,
	}
	now := time.Now()
	template.NotBefore = now.Add(NotBeforeJitter)
	template.NotAfter = now.AddDate(DefaultValidityYears, 0, 0)
	template.KeyUsage = x509.KeyUsageKeyEncipherment |
		x509.KeyUsageDigitalSignature |
		x509.KeyUsageCertSign
	template.BasicConstraintsValid = true
	template.IsCA = true

	der, err := x509.CreateCertificate(rand.Reader, template, template,
		signer.Public(), signer)
	if err != nil {
		return nil, errors.Annotate(err, "failed creating CA certificate")
	}

	caCert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return caCert, nil
}

// NewDefaultAuthority generates a new DefaultAuthority for the supplied CA
// cert and keys. Error is returned when the supplied certificate is not a CA.
func NewDefaultAuthority(authority *x509.Certificate, signer crypto.Signer,
	chain ...*x509.Certificate) (*DefaultAuthority, error) {
	if !authority.IsCA {
		return nil, errors.NotValidf("%s is not a certificate authority",
			authority.Subject)
	}

	return &DefaultAuthority{
		authority: NewDefaultLeaf("", authority, chain, signer),
	}, nil
}

// NewDefaultAuthorityPem generates a new DefaultAuthority for the supplied pem
// block. The pem block must contain a valid CA certificate and associated
// private key.
func NewDefaultAuthorityPem(pemBlock []byte) (*DefaultAuthority, error) {
	leaf, err := NewDefaultLeafPem("", pemBlock)
	if err != nil {
		return nil, errors.Annotate(err, "generating CA leaf")
	}

	if !leaf.Certificate().IsCA {
		return nil, errors.Errorf("certificate %s is not a CA",
			leaf.Certificate().Subject.CommonName)
	}

	return NewDefaultAuthority(leaf.Certificate(), leaf.Signer(), leaf.Chain()...)
}

// NewDefaultAuthorityPemCAKey generates a new DefaultAuthority for the supplied
// pem ca and key. Returns error if the supplied cert is not a ca or passing of
// the pem data fails.
func NewDefaultAuthorityPemCAKey(caPem, keyPem []byte) (*DefaultAuthority, error) {
	return NewDefaultAuthorityPem(append(caPem, keyPem...))
}

// SetLeafSigner sets a default signer to use for all new created leafs on this
// authority.
func (a *DefaultAuthority) SetLeafSigner(signer crypto.Signer) {
	a.leafSignerMutex.Lock()
	defer a.leafSignerMutex.Unlock()
	a.leafSigner = signer
}

// Signer implements Leaf interface method. Returns the signer used for this
// authority.
func (a *DefaultAuthority) Signer() crypto.Signer {
	return a.authority.Signer()
}

// TLSCertificate implements Leaf interface method. Returns a tls certificate
// that can be used in tls connections.
func (a *DefaultAuthority) TLSCertificate() *tls.Certificate {
	return a.authority.TLSCertificate()
}

// ToPemParts implements the Leaf interface method. Returns this authority split
// into certificate and key pem components.
func (a *DefaultAuthority) ToPemParts() (cert, key []byte, err error) {
	return a.authority.ToPemParts()
}
