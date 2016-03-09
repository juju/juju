// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"io"
	"io/ioutil"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/resource"
)

type stubRawState struct {
	stub *testing.Stub

	ReturnPersistence Persistence
	ReturnStorage     Storage
}

func (s *stubRawState) Persistence() Persistence {
	s.stub.AddCall("Persistence")
	s.stub.NextErr()

	return s.ReturnPersistence
}

func (s *stubRawState) Storage() Storage {
	s.stub.AddCall("Storage")
	s.stub.NextErr()

	return s.ReturnStorage
}

type stubPersistence struct {
	stub *testing.Stub

	ReturnListResources                resource.ServiceResources
	ReturnListPendingResources         []resource.Resource
	ReturnGetResource                  resource.Resource
	ReturnGetResourcePath              string
	ReturnStageResource                *stubStagedResource
	ReturnNewResolvePendingResourceOps [][]txn.Op

	CallsForNewResolvePendingResourceOps map[string]string
}

func (s *stubPersistence) ListResources(serviceID string) (resource.ServiceResources, error) {
	s.stub.AddCall("ListResources", serviceID)
	if err := s.stub.NextErr(); err != nil {
		return resource.ServiceResources{}, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}

func (s *stubPersistence) ListPendingResources(serviceID string) ([]resource.Resource, error) {
	s.stub.AddCall("ListPendingResources", serviceID)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListPendingResources, nil
}

func (s *stubPersistence) GetResource(serviceID string) (resource.Resource, string, error) {
	s.stub.AddCall("GetResource", serviceID)
	if err := s.stub.NextErr(); err != nil {
		return resource.Resource{}, "", errors.Trace(err)
	}

	return s.ReturnGetResource, s.ReturnGetResourcePath, nil
}

func (s *stubPersistence) StageResource(res resource.Resource, storagePath string) (StagedResource, error) {
	s.stub.AddCall("StageResource", res, storagePath)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnStageResource, nil
}

func (s *stubPersistence) SetResource(res resource.Resource) error {
	s.stub.AddCall("SetResource", res)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubPersistence) SetCharmStoreResource(id, serviceID string, chRes charmresource.Resource, lastPolled time.Time) error {
	s.stub.AddCall("SetCharmStoreResource", id, serviceID, chRes, lastPolled)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubPersistence) SetUnitResource(unitID string, res resource.Resource) error {
	s.stub.AddCall("SetUnitResource", unitID, res)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubPersistence) NewResolvePendingResourceOps(resID, pendingID string) ([]txn.Op, error) {
	s.stub.AddCall("NewResolvePendingResourceOps", resID, pendingID)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	if s.CallsForNewResolvePendingResourceOps == nil {
		s.CallsForNewResolvePendingResourceOps = make(map[string]string)
	}
	s.CallsForNewResolvePendingResourceOps[resID] = pendingID

	if len(s.ReturnNewResolvePendingResourceOps) == 0 {
		return nil, nil
	}
	ops := s.ReturnNewResolvePendingResourceOps[0]
	s.ReturnNewResolvePendingResourceOps = s.ReturnNewResolvePendingResourceOps[1:]
	return ops, nil
}

type stubStagedResource struct {
	stub *testing.Stub
}

func (s *stubStagedResource) Unstage() error {
	s.stub.AddCall("Unstage")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubStagedResource) Activate() error {
	s.stub.AddCall("Activate")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type stubStorage struct {
	stub *testing.Stub

	ReturnGet resource.Content
}

func (s *stubStorage) PutAndCheckHash(path string, r io.Reader, length int64, hash string) error {
	s.stub.AddCall("PutAndCheckHash", path, r, length, hash)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubStorage) Get(path string) (io.ReadCloser, int64, error) {
	s.stub.AddCall("Get", path)
	if err := s.stub.NextErr(); err != nil {
		return nil, 0, errors.Trace(err)
	}

	if readCloser, ok := s.ReturnGet.Data.(io.ReadCloser); ok {
		return readCloser, s.ReturnGet.Size, nil
	}
	return ioutil.NopCloser(s.ReturnGet.Data), s.ReturnGet.Size, nil
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
		return 0, err
	}

	return s.ReturnRead, nil
}
