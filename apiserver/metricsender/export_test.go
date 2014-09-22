// Copyright 2014 Canonical Ltd.  All rights reserved.

package metricsender

import (
	"crypto/x509"

	"github.com/juju/testing"
)

func PatchHostAndCertPool(host string, certPool *x509.CertPool) func() {
	restoreHost := testing.PatchValue(&metricsHost, host)
	restoreCertsPool := testing.PatchValue(&metricsCertsPool, certPool)
	return func() {
		restoreHost()
		restoreCertsPool()
	}
}
