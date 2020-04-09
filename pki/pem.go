// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki

import (
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"io"
	"strings"

	"github.com/juju/errors"
)

const (
	PEMTypeCertificate = "CERTIFICATE"
	PEMTypePKCS1       = "RSA PRIVATE KEY"
	PEMTypePKCS8       = "PRIVATE KEY"
)

var (
	DefaultPemHeaders = map[string]string{}
	hexAlphabet       = []byte("0123456789ABCDEF")
)

// CertificateToPemString transforms an x509 certificate to a pem string
func CertificateToPemString(headers map[string]string,
	cert *x509.Certificate, chain ...*x509.Certificate) (string, error) {
	builder := strings.Builder{}
	if err := CertificateToPemWriter(&builder, headers, cert, chain...); err != nil {
		return "", errors.Trace(err)
	}
	return builder.String(), nil
}

// CertificateToPemWriter transforms an x509 certificate to pem format on the
// supplied writer
func CertificateToPemWriter(writer io.Writer, headers map[string]string,
	cert *x509.Certificate, chain ...*x509.Certificate) error {
	for _, cert := range append([]*x509.Certificate{cert}, chain...) {
		err := pem.Encode(writer, &pem.Block{
			Bytes: cert.Raw,
			//TODO re-enable headers on certificate to pem when Juju upgrade
			//CAAS mongo to something compiled with latest openssl. Currently
			//not all our Openssl versions support pem headers. Make sure to
			//also uncomment test.
			//Headers: headers,
			Type: PEMTypeCertificate,
		})
		if err != nil {
			return errors.Annotate(err, "encoding certificate to pem format")
		}
	}
	return nil
}

// Fingerprint returns a human-readable SHA-256 fingerprint for a certificate
// stored in the PEM format. The returned fingerprint matches the output of:
// openssl x509 -noout -fingerprint -sha256 -inform pem -in cert.pem. Also
// returns the remainder of the input for the next blocks.
func Fingerprint(pemData []byte) (string, []byte, error) {
	block, rest := pem.Decode(pemData)
	if block == nil {
		return "", rest, errors.New(
			"input does not contain a valid certificate in PEM format")
	} else if block.Type != PEMTypeCertificate {
		return "", rest, errors.NotValidf(
			"discovered pem block is not of type %s", PEMTypeCertificate)
	}

	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		return "", rest, errors.Annotate(err, "cannot parse pem certificate to x509")
	}

	// fingerprint format is: XX:YY:...:ZZ
	fingerprint := make([]byte, (sha256.Size*3)-1)
	var index int
	for _, fb := range sha256.Sum256(block.Bytes) {
		if index != 0 {
			fingerprint[index] = ':'
			index++
		}

		// Encode each byte as two chars
		fingerprint[index] = hexAlphabet[(fb>>4)&0xf]
		fingerprint[index+1] = hexAlphabet[fb&0xf]
		index += 2
	}

	return string(fingerprint), rest, nil
}

// IsPemCA returns true if the supplied pem certificate is a CA
func IsPemCA(pemData []byte) (bool, error) {
	certs, _, err := UnmarshalPemData(pemData)
	if err != nil {
		return false, errors.Trace(err)
	}
	if len(certs) == 0 {
		return false, errors.New("no certificates in pem bundle")
	}
	return certs[0].IsCA, nil
}

// SignerToPemString transforms a crypto signer to PKCS8 pem string
func SignerToPemString(signer crypto.Signer) (string, error) {
	builder := strings.Builder{}
	if err := SignerToPemWriter(&builder, signer); err != nil {
		return "", errors.Trace(err)
	}
	return builder.String(), nil
}

// SignerToPemWriter transforms a crypto signer to PKCS8 pem using the supplied
// writer
func SignerToPemWriter(writer io.Writer, signer crypto.Signer) error {
	der, err := x509.MarshalPKCS8PrivateKey(signer)
	if err != nil {
		return errors.Annotate(err, "marshalling signer to pkcs8 format")
	}

	err = pem.Encode(writer, &pem.Block{
		Type:  PEMTypePKCS8,
		Bytes: der,
	})
	if err != nil {
		return errors.Annotate(err, "encoding signer to pkcs8 pem format")
	}
	return nil
}

// UnmarshalSignerFromPemBlock transforms a given pem block to a crypto signer
func UnmarshalSignerFromPemBlock(block *pem.Block) (crypto.Signer, error) {
	switch blockType := block.Type; blockType {
	case PEMTypePKCS8:
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.Annotate(err, "parsing pem private key")
		}
		signer, ok := key.(crypto.Signer)
		if !ok {
			return nil, errors.New("unable to case pem private key to crypto.Signer")
		}
		return signer, nil
	case PEMTypePKCS1:
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.Annotate(err, "parsing pem private key")
		}
		return key, nil
	default:
		return nil, errors.NotSupportedf("block type %s", blockType)
	}
}

// UnmarshalPemData unmarshals a set of pem data into certificates and signers
func UnmarshalPemData(pemData []byte) ([]*x509.Certificate, []crypto.Signer, error) {
	var (
		block *pem.Block
		rest  = pemData
	)

	certificates := []*x509.Certificate{}
	signers := []crypto.Signer{}

	for {
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		switch blockType := block.Type; blockType {
		case PEMTypeCertificate:
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, nil, errors.Annotate(err, "parsing pem certificate block")
			}
			certificates = append(certificates, cert)
		case PEMTypePKCS8:
			signer, err := UnmarshalSignerFromPemBlock(block)
			if err != nil {
				return nil, nil, errors.Annotate(err, "parsing pem private key block")
			}
			signers = append(signers, signer)
		case PEMTypePKCS1:
			signer, err := UnmarshalSignerFromPemBlock(block)
			if err != nil {
				return nil, nil, errors.Annotate(err, "parsing pem private key block")
			}
			signers = append(signers, signer)
		default:
			return nil, nil, errors.NotSupportedf("block type %s", blockType)
		}
	}
	return certificates, signers, nil
}
