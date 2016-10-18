// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package binarystorage_test

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/binarystorage"
	coretesting "github.com/juju/juju/testing"
)

type layeredStorageSuite struct {
	coretesting.BaseSuite
	stores []*mockStorage
	store  binarystorage.Storage
}

var _ = gc.Suite(&layeredStorageSuite{})

func (s *layeredStorageSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.stores = []*mockStorage{{
		metadata: []binarystorage.Metadata{{
			Version: "1.0", Size: 1, SHA256: "foo",
		}, {
			Version: "2.0", Size: 2, SHA256: "bar",
		}},
	}, {
		metadata: []binarystorage.Metadata{{
			Version: "3.0", Size: 3, SHA256: "baz",
		}, {
			Version: "1.0", Size: 3, SHA256: "meh",
		}},
	}}

	stores := make([]binarystorage.Storage, len(s.stores))
	for i, store := range s.stores {
		stores[i] = store
	}
	var err error
	s.store, err = binarystorage.NewLayeredStorage(stores...)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *layeredStorageSuite) TestNewLayeredStorageError(c *gc.C) {
	_, err := binarystorage.NewLayeredStorage(s.stores[0])
	c.Assert(err, gc.ErrorMatches, "expected multiple stores")
}

func (s *layeredStorageSuite) TestAdd(c *gc.C) {
	r := new(readCloser)
	m := binarystorage.Metadata{Version: "4.0", Size: 4, SHA256: "qux"}
	expectedErr := errors.New("wut")
	s.stores[0].SetErrors(expectedErr)
	err := s.store.Add(r, m)
	c.Assert(err, gc.Equals, expectedErr)
	s.stores[0].CheckCalls(c, []testing.StubCall{{"Add", []interface{}{r, m}}})
	s.stores[1].CheckNoCalls(c)
}

func (s *layeredStorageSuite) TestAllMetadata(c *gc.C) {
	all, err := s.store.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, jc.DeepEquals, []binarystorage.Metadata{
		{Version: "1.0", Size: 1, SHA256: "foo"},
		{Version: "2.0", Size: 2, SHA256: "bar"},
		{Version: "3.0", Size: 3, SHA256: "baz"},
	})
	s.stores[0].CheckCallNames(c, "AllMetadata")
	s.stores[1].CheckCallNames(c, "AllMetadata")
}

func (s *layeredStorageSuite) TestAllMetadataError(c *gc.C) {
	expectedErr := errors.New("wut")
	s.stores[0].SetErrors(expectedErr)
	_, err := s.store.AllMetadata()
	c.Assert(err, gc.Equals, expectedErr)
	s.stores[0].CheckCallNames(c, "AllMetadata")
	s.stores[1].CheckNoCalls(c)
}

func (s *layeredStorageSuite) TestMetadata(c *gc.C) {
	s.stores[0].SetErrors(errors.NotFoundf("metadata"))
	m, err := s.store.Metadata("3.0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, jc.DeepEquals, s.stores[1].metadata[0])
	s.stores[0].CheckCalls(c, []testing.StubCall{{
		"Metadata", []interface{}{"3.0"},
	}})
	s.stores[1].CheckCalls(c, []testing.StubCall{{
		"Metadata", []interface{}{"3.0"},
	}})
}

func (s *layeredStorageSuite) TestMetadataEarlyExit(c *gc.C) {
	m, err := s.store.Metadata("1.0")
	c.Assert(err, jc.ErrorIsNil)
	s.stores[0].CheckCalls(c, []testing.StubCall{{
		"Metadata", []interface{}{"1.0"},
	}})
	s.stores[1].CheckNoCalls(c)
	c.Assert(m, jc.DeepEquals, s.stores[0].metadata[0])
}

func (s *layeredStorageSuite) TestMetadataFatalError(c *gc.C) {
	expectedErr := errors.New("wut")
	s.stores[0].SetErrors(expectedErr)
	_, err := s.store.Metadata("1.0")
	c.Assert(err, gc.Equals, expectedErr)
	s.stores[0].CheckCalls(c, []testing.StubCall{{
		"Metadata", []interface{}{"1.0"},
	}})
	s.stores[1].CheckNoCalls(c)
}

func (s *layeredStorageSuite) TestOpen(c *gc.C) {
	s.stores[0].SetErrors(errors.NotFoundf("metadata"))
	m, rc, err := s.store.Open("3.0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, jc.DeepEquals, s.stores[1].metadata[0])
	c.Assert(rc, gc.Equals, &s.stores[1].rc)
	s.stores[0].CheckCalls(c, []testing.StubCall{{
		"Open", []interface{}{"3.0"},
	}})
	s.stores[1].CheckCalls(c, []testing.StubCall{{
		"Open", []interface{}{"3.0"},
	}})
}

func (s *layeredStorageSuite) TestOpenEarlyExit(c *gc.C) {
	m, rc, err := s.store.Open("1.0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, jc.DeepEquals, s.stores[0].metadata[0])
	c.Assert(rc, gc.Equals, &s.stores[0].rc)
	s.stores[0].CheckCalls(c, []testing.StubCall{{
		"Open", []interface{}{"1.0"},
	}})
	s.stores[1].CheckNoCalls(c)
}

func (s *layeredStorageSuite) TestOpenFatalError(c *gc.C) {
	expectedErr := errors.New("wut")
	s.stores[0].SetErrors(expectedErr)
	_, _, err := s.store.Open("1.0")
	c.Assert(err, gc.Equals, expectedErr)
	s.stores[0].CheckCalls(c, []testing.StubCall{{
		"Open", []interface{}{"1.0"},
	}})
	s.stores[1].CheckNoCalls(c)
}

type mockStorage struct {
	testing.Stub
	rc       readCloser
	metadata []binarystorage.Metadata
}

func (s *mockStorage) Add(r io.Reader, m binarystorage.Metadata) error {
	s.MethodCall(s, "Add", r, m)
	return s.NextErr()
}

func (s *mockStorage) AllMetadata() ([]binarystorage.Metadata, error) {
	s.MethodCall(s, "AllMetadata")
	return s.metadata, s.NextErr()
}

func (s *mockStorage) Metadata(version string) (binarystorage.Metadata, error) {
	s.MethodCall(s, "Metadata", version)
	return s.metadata[0], s.NextErr()
}

func (s *mockStorage) Open(version string) (binarystorage.Metadata, io.ReadCloser, error) {
	s.MethodCall(s, "Open", version)
	return s.metadata[0], &s.rc, s.NextErr()
}

type readCloser struct{ io.ReadCloser }
