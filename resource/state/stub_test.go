// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/state"
)

type stubRawState struct {
	stub *testing.Stub

	ReturnPersistence state.Persistence
	ReturnStorage     state.Storage
}

func (s *stubRawState) Persistence() state.Persistence {
	s.stub.AddCall("Persistence")
	s.stub.NextErr()

	return s.ReturnPersistence
}

func (s *stubRawState) Storage() state.Storage {
	s.stub.AddCall("Storage")
	s.stub.NextErr()

	return s.ReturnStorage
}

type stubPersistence struct {
	stub *testing.Stub

	ReturnListResources []resource.Resource
}

func (s *stubPersistence) ListResources(serviceID string) ([]resource.Resource, error) {
	s.stub.AddCall("ListResources", serviceID)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}

func (s *stubPersistence) StageResource(id, serviceID string, res resource.Resource) error {
	s.stub.AddCall("StageResource", id, serviceID, res)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubPersistence) UnstageResource(id, serviceID string) error {
	s.stub.AddCall("UnstageResource", id, serviceID)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubPersistence) SetResource(id, serviceID string, res resource.Resource) error {
	s.stub.AddCall("SetResource", id, serviceID, res)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type stubStorage struct {
	stub *testing.Stub
}

func (s *stubStorage) PutAndCheckHash(path string, r io.Reader, length int64, hash string) error {
	s.stub.AddCall("PutAndCheckHash", path, r, length, hash)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubStorage) Remove(path string) error {
	s.stub.AddCall("Remove", path)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type stubReader struct {
	stub *testing.Stub

	ReturnRead int
}

func (s *stubReader) Read(buf []byte) (int, error) {
	s.stub.AddCall("Read", buf)
	if err := s.stub.NextErr(); err != nil {
		return 0, errors.Trace(err)
	}

	return s.ReturnRead, nil
}
