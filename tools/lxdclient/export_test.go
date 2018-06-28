// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"github.com/juju/testing"
)

func PatchGenerateCertificate(s *testing.CleanupSuite, cert, key string) {
	s.PatchValue(&generateCertificate, func() ([]byte, []byte, error) {
		return []byte(cert), []byte(key), nil
	})
}
