// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ovatest

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
)

var (
	fakeOva       []byte
	fakeOvaSha256 string
)

// FakeOVAContents returns the contents of a fake OVA file.
func FakeOVAContents() []byte {
	ova := make([]byte, len(fakeOva))
	copy(ova, fakeOva)
	return ova
}

// FakeOVASHA256 returns the hex-encoded SHA-256 hash of the
// OVA contents as returned by FakeOVAContents.
func FakeOVASHA256() string {
	return fakeOvaSha256
}

func init() {
	buf := new(bytes.Buffer)
	hash := sha256.New()
	tw := tar.NewWriter(io.MultiWriter(buf, hash))
	var files = []struct{ Name, Body string }{
		{"ubuntu-14.04-server-cloudimg-amd64.ovf", "FakeOvfContent"},
		{"ubuntu-14.04-server-cloudimg-amd64.vmdk", "FakeVmdkContent"},
	}
	for _, file := range files {
		hdr := &tar.Header{
			Name: file.Name,
			Size: int64(len(file.Body)),
		}
		_ = tw.WriteHeader(hdr)
		_, _ = tw.Write([]byte(file.Body))
	}
	tw.Close()
	fakeOva = buf.Bytes()
	fakeOvaSha256 = fmt.Sprintf("%x", hash.Sum(nil))
}
