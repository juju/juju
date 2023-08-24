// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package assertion

import (
	"crypto/x509"
)

// HasExtKeyUsage checks the supplied certificates extended key usages to see if
// has is signed into the certificate. Performs no validation on the certificate
// expect for checking the ExtKeyUsage field.
func HasExtKeyUsage(cert *x509.Certificate, has x509.ExtKeyUsage) bool {
	for _, usage := range cert.ExtKeyUsage {
		if usage == has {
			return true
		}
	}

	return false
}
