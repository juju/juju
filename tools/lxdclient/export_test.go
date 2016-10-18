// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/testing"
)

var NewInstanceSummary = newInstanceSummary

type RawInstanceClient rawInstanceClient

func NewInstanceClient(raw RawInstanceClient) *instanceClient {
	return &instanceClient{
		raw:    rawInstanceClient(raw),
		remote: "",
	}
}

func PatchGenerateCertificate(s *testing.CleanupSuite, cert, key string) {
	s.PatchValue(&generateCertificate, func() ([]byte, []byte, error) {
		return []byte(cert), []byte(key), nil
	})
}
