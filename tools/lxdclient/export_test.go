// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/testing"
)

var NewInstanceSummary = newInstanceSummary

type (
	RawInstanceClient rawInstanceClient
	RawStorageClient  rawStorageClient
)

func NewInstanceClient(raw RawInstanceClient) *instanceClient {
	return &instanceClient{
		raw:    rawInstanceClient(raw),
		remote: "",
	}
}

func NewStorageClient(raw RawStorageClient, supported bool) *storageClient {
	return &storageClient{
		raw:       raw,
		supported: supported,
	}
}

func PatchGenerateCertificate(s *testing.CleanupSuite, cert, key string) {
	s.PatchValue(&generateCertificate, func() ([]byte, []byte, error) {
		return []byte(cert), []byte(key), nil
	})
}
