// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2016 Cloudbase solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package cert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/juju/errors"
)

// OtherName type for asn1 encoding
type OtherName struct {
	A string `asn1:"utf8"`
}

// GeneralName type for asn1 encoding
type GeneralName struct {
	OID       asn1.ObjectIdentifier
	OtherName `asn1:"tag:0"`
}

// GeneralNames type for asn1 encoding
type GeneralNames struct {
	GeneralName `asn1:"tag:0"`
}

// Config type used for specifing different params for NewLeaf func
// This will effect the generation of certificates.
type Config struct {
	CommonName  string             // CommonName common name of the certificate
	UUID        string             // UUID for a specific model
	Expiry      time.Time          // Expiry when the certificate will expire
	CA          []byte             // CA certifiacte authority to add a new leaf cert to it
	CAKey       []byte             // CAKey private key of the CA to add a new leaf cert to it
	IsCA        bool               // IsCA if we want to generate new a CA cert
	Hostnames   []string           // Hostnames , list of hostnames for the certificate
	ExtKeyUsage []x509.ExtKeyUsage // ExtKeyUsage extra flags for special usage of the cert
	KeyBits     int                // KeyBits is used to set the lenght of the RSA key, default value 2048 bytes
	Client      bool               // generate client certificate for certificate authentication
}

// NewLeaf generates a certificate/key pair suitable for use
// by a server, leaf node, client authentication, etc.
// In order to generate certs for multiple purposes please consult the Config type.
func NewLeaf(cfg *Config) (certPEM, keyPEM string, err error) {
	var (
		caCert *x509.Certificate
		caKey  *rsa.PrivateKey
	)

	if cfg.CA != nil && cfg.CAKey != nil && !cfg.IsCA {
		tlsCert, err := tls.X509KeyPair(cfg.CA, cfg.CAKey)
		if err != nil {
			return "", "", errors.Trace(err)
		}
		if len(tlsCert.Certificate) != 1 {
			return "", "", fmt.Errorf("more than one certificate for CA")
		}

		caCert, err = x509.ParseCertificate(tlsCert.Certificate[0])
		if err != nil {
			return "", "", errors.Trace(err)
		}
		if !caCert.BasicConstraintsValid || !caCert.IsCA {
			return "", "", errors.Errorf("CA certificate is not a valid CA")
		}
		var ok bool
		caKey, ok = tlsCert.PrivateKey.(*rsa.PrivateKey)
		if !ok {
			return "", "", errors.Errorf("CA private key has unexpected type %T", tlsCert.PrivateKey)
		}
	}

	// if none assign default
	if cfg.KeyBits == 0 {
		cfg.KeyBits = 2048
	}

	// generate private key
	key, err := rsa.GenerateKey(rand.Reader, cfg.KeyBits)
	if err != nil {
		return "", "", errors.Errorf("cannot generate key: %v", err)
	}
	serialNumber, err := newSerialNumber()
	if err != nil {
		return "", "", errors.Trace(err)
	}
	subject := pkix.Name{
		CommonName:   cfg.CommonName,
		Organization: []string{"juju"},
		SerialNumber: cfg.UUID,
	}

	var value []byte
	// get asn1 encoded info of the subject pkix
	if cfg.Client {
		value, err = getUPNExtensionValue(subject)
		if err != nil {
			return "", "", fmt.Errorf("Can't marshal asn1 encoded %s", err)
		}
	}

	// TODO(perrito666) 2016-05-02 lp:1558657
	now := time.Now()
	template := &x509.Certificate{
		Subject:      subject,
		SerialNumber: serialNumber,
		NotBefore:    now.UTC().AddDate(0, 0, -7),
		Version:      2,
		NotAfter:     cfg.Expiry.UTC(),
		SubjectKeyId: bigIntHash(key.N),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyAgreement,
		ExtKeyUsage:  cfg.ExtKeyUsage,
	}

	if cfg.Client {
		template.ExtraExtensions = []pkix.Extension{
			{Id: subjAltName, Critical: false, Value: value},
		}
	}

	if cfg.IsCA {
		template.BasicConstraintsValid = true
		template.IsCA = true
		template.KeyUsage = x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign
	}

	for _, hostname := range cfg.Hostnames {
		if ip := net.ParseIP(hostname); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, hostname)
		}
	}

	if caKey == nil && caCert == nil {
		caCert = template
		caKey = key
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, getPublicKey(key), caKey)
	if err != nil {
		return "", "", err
	}

	certPEMData := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyPEMData := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return string(certPEMData), string(keyPEMData), nil
}

var (
	// https://support.microsoft.com/en-us/kb/287547
	//  szOID_NT_PRINCIPAL_NAME 1.3.6.1.4.1.311.20.2.3
	szOID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 20, 2, 3}
	// http://www.umich.edu/~x509/ssleay/asn1-oids.html
	// 2 5 29 17  subjectAltName
	subjAltName = asn1.ObjectIdentifier{2, 5, 29, 17}
)

// NewCA generates a CA certificate/key pair suitable for signing server
// keys for an environment with the given name.
func NewCA(commonName, UUID string, expiry time.Time, keyBits int) (certPEM, keyPEM string, err error) {
	certPEM, keyPEM, err = NewLeaf(&Config{
		CommonName: commonName,
		UUID:       UUID,
		Expiry:     expiry,
		IsCA:       true,
		KeyBits:    keyBits,
	})
	if err != nil {
		return "", "", errors.Annotatef(err, "cannot generate ca certificate")
	}
	return
}

// NewClientCert generates a x509 client certificate used for https authentication sessions.
func NewClientCert(commonName, UUID string, expiry time.Time, keyBits int) (certPEM string, keyPEM string, err error) {
	certPEM, keyPEM, err = NewLeaf(&Config{
		CommonName:  commonName,
		UUID:        UUID,
		Expiry:      expiry,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyBits:     keyBits,
		Client:      true,
	})
	if err != nil {
		return "", "", errors.Annotatef(err, "Cannot generate client certificate")
	}

	return
}
func bigIntHash(n *big.Int) []byte {
	h := sha1.New()
	h.Write(n.Bytes())
	return h.Sum(nil)
}

// getUPNExtensionValue returns marsheled asn1 encoded info
func getUPNExtensionValue(subject pkix.Name) ([]byte, error) {
	// returns the ASN.1 encoding of val
	// in addition to the struct tags recognized
	// we used:
	// utf8 => causes string to be marsheled as ASN.1, UTF8 strings
	// tag:x => specifies the ASN.1 tag number; imples ASN.1 CONTEXT SPECIFIC
	return asn1.Marshal(GeneralNames{
		GeneralName: GeneralName{
			// init our ASN.1 object identifier
			OID: szOID,
			OtherName: OtherName{
				A: subject.CommonName,
			},
		},
	})
}

// newSerialNumber returns a new random serial number suitable
// for use in a certificate.
func newSerialNumber() (*big.Int, error) {
	// A serial number can be up to 20 octets in size.
	// https://tools.ietf.org/html/rfc5280#section-4.1.2.2
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 8*20))
	if err != nil {
		return nil, errors.Annotatef(err, "failed to generate serial number")
	}
	return n, nil
}

// getPublicKey fetch public key from a PrivateKey type
// this will return nil if the key is not RSA type
func getPublicKey(p interface{}) interface{} {
	switch t := p.(type) {
	case *rsa.PrivateKey:
		return t.Public()
	default:
		return nil
	}
}

// ParseCert parses the given PEM-formatted X509 certificate.
func ParseCert(certPEM string) (*x509.Certificate, error) {
	certPEMData := []byte(certPEM)
	for len(certPEMData) > 0 {
		var certBlock *pem.Block
		certBlock, certPEMData = pem.Decode(certPEMData)
		if certBlock == nil {
			break
		}
		if certBlock.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(certBlock.Bytes)
			return cert, err
		}
	}
	return nil, errors.New("no certificates found")
}

// ParseCertAndKey parses the given PEM-formatted X509 certificate
// and RSA private key.
func ParseCertAndKey(certPEM, keyPEM string) (*x509.Certificate, *rsa.PrivateKey, error) {
	tlsCert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, nil, err
	}

	key, ok := tlsCert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("private key with unexpected type %T", key)
	}
	return cert, key, nil
}
