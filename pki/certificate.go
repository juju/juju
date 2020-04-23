// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki

import (
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"

	"github.com/juju/errors"
	"github.com/juju/utils"
)

// CSRToCertificate copies all fields from a CertificateRequest into a new x509
// Certificate. No policy check is performed this is just a straight 1 to 1
// copy.
func CSRToCertificate(csr *x509.CertificateRequest) *x509.Certificate {
	cert := &x509.Certificate{}

	cert.Subject = csr.Subject
	cert.Extensions = csr.Extensions
	cert.ExtraExtensions = csr.ExtraExtensions
	cert.DNSNames = csr.DNSNames
	cert.EmailAddresses = csr.EmailAddresses
	cert.IPAddresses = csr.IPAddresses
	cert.URIs = csr.URIs

	return cert
}

func assetTagCertificate(cert *x509.Certificate) error {
	uuid, err := utils.NewUUID()
	if err != nil {
		return errors.Annotate(err, "failed to generate new certificate uuid")
	}

	serialNumber, err := newSerialNumber()
	if err != nil {
		return errors.Annotate(err,
			"failed to generate new certificate serial number")
	}

	cert.SerialNumber = serialNumber
	cert.Subject.SerialNumber = uuid.String()
	return nil
}

func bigIntHash(n *big.Int) []byte {
	h := sha1.New()
	h.Write(n.Bytes())
	return h.Sum(nil)
}

// MakeX509NameFromDefaults constructs a new x509 name from the merging of a
// default and request name. Fields not set in the request name
// will be copied from the default name.
func MakeX509NameFromDefaults(template, request *pkix.Name) pkix.Name {
	rval := pkix.Name{}
	if template == nil {
		template = &pkix.Name{}
	}

	rval.Country = request.Country
	if len(rval.Country) == 0 {
		rval.Country = template.Country
	}

	rval.Organization = request.Organization
	if len(rval.Organization) == 0 {
		rval.Organization = template.Organization
	}

	rval.OrganizationalUnit = request.OrganizationalUnit
	if len(rval.OrganizationalUnit) == 0 {
		rval.OrganizationalUnit = template.OrganizationalUnit
	}

	rval.Locality = request.Locality
	if len(rval.Locality) == 0 {
		rval.Locality = template.Locality
	}

	rval.Province = request.Province
	if len(rval.Province) == 0 {
		rval.Province = template.Province
	}

	rval.StreetAddress = request.StreetAddress
	if len(rval.StreetAddress) == 0 {
		rval.StreetAddress = template.StreetAddress
	}

	rval.PostalCode = request.PostalCode
	if len(rval.PostalCode) == 0 {
		rval.PostalCode = template.PostalCode
	}

	rval.SerialNumber = request.SerialNumber
	if rval.SerialNumber == "" {
		rval.SerialNumber = template.SerialNumber
	}

	rval.CommonName = request.CommonName
	if rval.CommonName == "" {
		rval.CommonName = template.CommonName
	}

	rval.Names = request.Names
	if len(rval.Names) == 0 {
		rval.Names = template.Names
	}

	rval.ExtraNames = request.ExtraNames
	if len(rval.ExtraNames) == 0 {
		rval.ExtraNames = template.ExtraNames
	}

	return rval
}

// newSerialNumber returns  a new random serial number suitable for use in a
// certificate.
func newSerialNumber() (*big.Int, error) {
	// A serial number can be up to 20 octets in size.
	// https://tools.ietf.org/html/rfc5280#section-4.1.2.2
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 8*20))
	if err != nil {
		return nil, errors.Annotatef(err, "failed to generate serial number")
	}
	return n, nil
}
