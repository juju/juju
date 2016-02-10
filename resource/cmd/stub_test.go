// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/testing"
)

type stubAPIClient struct {
	stub *testing.Stub
}

func (s *stubAPIClient) Upload(service, name string, resource io.ReadSeeker) error {
	s.stub.AddCall("Upload", service, name, resource)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubAPIClient) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type stubFile struct {
	// No one actually tries to read from this during tests.
	io.ReadSeeker
	stub *testing.Stub
}

func (s *stubFile) Close() error {
	s.stub.AddCall("FileClose")
	return errors.Trace(s.stub.NextErr())
}
