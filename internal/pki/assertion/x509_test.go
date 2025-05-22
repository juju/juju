// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package assertion_test

import (
	"crypto/x509"
	stdtesting "testing"

	"github.com/juju/juju/internal/pki/assertion"
)

func TestHasExtKeyUsage(t *stdtesting.T) {
	tests := []struct {
		Name        string
		ExtKeyUsage []x509.ExtKeyUsage
		CheckKey    x509.ExtKeyUsage
		Expected    bool
	}{
		{
			Name:     "Test Empty ExtKeyUsage",
			CheckKey: x509.ExtKeyUsageServerAuth,
			Expected: false,
		},
		{
			Name: "Test Single ExtKeyUsage",
			ExtKeyUsage: []x509.ExtKeyUsage{
				x509.ExtKeyUsageServerAuth,
			},
			CheckKey: x509.ExtKeyUsageServerAuth,
			Expected: true,
		},
		{
			Name: "Test Single ExtKeyUsage Not Found",
			ExtKeyUsage: []x509.ExtKeyUsage{
				x509.ExtKeyUsageTimeStamping,
			},
			CheckKey: x509.ExtKeyUsageServerAuth,
			Expected: false,
		},
		{
			Name: "Test Multiple ExtKeyUsage",
			ExtKeyUsage: []x509.ExtKeyUsage{
				x509.ExtKeyUsageTimeStamping,
				x509.ExtKeyUsageNetscapeServerGatedCrypto,
				x509.ExtKeyUsageServerAuth,
				x509.ExtKeyUsageMicrosoftKernelCodeSigning,
			},
			CheckKey: x509.ExtKeyUsageServerAuth,
			Expected: true,
		},
	}

	for _, test := range tests {
		_ = t.Run(test.Name, func(t *stdtesting.T) {
			rval := assertion.HasExtKeyUsage(&x509.Certificate{
				ExtKeyUsage: test.ExtKeyUsage,
			}, test.CheckKey)

			if rval == test.Expected {
				return
			}

			t.Errorf("expected result %t for ExtKeyUsage(%v) and has %v",
				test.Expected,
				test.ExtKeyUsage,
				test.CheckKey,
			)
		})
	}
}
