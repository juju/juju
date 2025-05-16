// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"io"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type maasStorageSuite struct {
	maasSuite
}

func TestMaasStorageSuite(t *stdtesting.T) { tc.Run(t, &maasStorageSuite{}) }
func makeCall(funcName string, args ...interface{}) testhelpers.StubCall {
	return testhelpers.StubCall{funcName, args}
}

func checkCalls(c *tc.C, stub *testhelpers.Stub, calls ...testhelpers.StubCall) {
	stub.CheckCalls(c, calls)
}

func (s *maasStorageSuite) makeStorage(c *tc.C, controller gomaasapi.Controller) *maasStorage {
	env := s.makeEnviron(c, controller)
	env.uuid = "prefix"
	storage, ok := NewStorage(env).(*maasStorage)
	c.Assert(ok, tc.IsTrue)
	return storage
}

func (s *maasStorageSuite) TestGetNoSuchFile(c *tc.C) {
	storage := s.makeStorage(c, newFakeControllerWithErrors(
		errors.New("This file no existence"),
	))
	_, err := storage.Get("grasshopper.avi")
	c.Assert(err, tc.ErrorMatches, "This file no existence")
}

func (s *maasStorageSuite) TestGetReadFails(c *tc.C) {
	storage := s.makeStorage(c, newFakeControllerWithFiles(
		&fakeFile{name: "prefix-grasshopper.avi", error: errors.New("read error")},
	))
	_, err := storage.Get("grasshopper.avi")
	c.Assert(err, tc.ErrorMatches, "read error")
}

func (s *maasStorageSuite) TestGetNotFound(c *tc.C) {
	storage := s.makeStorage(c, newFakeControllerWithErrors(
		gomaasapi.NewNoMatchError("wee"),
	))
	_, err := storage.Get("grasshopper.avi")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *maasStorageSuite) TestGetSuccess(c *tc.C) {
	controller := newFakeControllerWithFiles(
		&fakeFile{name: "prefix-grasshopper.avi", contents: []byte("The man in the high castle")},
	)
	storage := s.makeStorage(c, controller)
	reader, err := storage.Get("grasshopper.avi")
	c.Assert(err, tc.ErrorIsNil)
	defer reader.Close()
	result, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []byte("The man in the high castle"))
	controller.Stub.CheckCall(c, 0, "GetFile", "prefix-grasshopper.avi")
}

func (s *maasStorageSuite) TestListError(c *tc.C) {
	storage := s.makeStorage(c, newFakeControllerWithErrors(
		errors.New("couldn't list files"),
	))
	_, err := storage.List("american-territories")
	c.Assert(err, tc.ErrorMatches, "couldn't list files")
}

func (s *maasStorageSuite) TestListSuccess(c *tc.C) {
	controller := newFakeControllerWithFiles(
		&fakeFile{name: "prefix-julianna"},
		&fakeFile{name: "prefix-frank"},
	)
	storage := s.makeStorage(c, controller)
	result, err := storage.List("grasshopper")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []string{"frank", "julianna"})
	controller.Stub.CheckCall(c, 0, "Files", "prefix-grasshopper")
}

func (s *maasStorageSuite) TestURLError(c *tc.C) {
	controller := newFakeControllerWithErrors(errors.New("no such file"))
	storage := s.makeStorage(c, controller)
	_, err := storage.URL("grasshopper.avi")
	c.Assert(err, tc.ErrorMatches, "no such file")
}

func (s *maasStorageSuite) TestURLSuccess(c *tc.C) {
	controller := newFakeControllerWithFiles(
		&fakeFile{name: "prefix-grasshopper.avi", url: "heavy lies"},
	)
	storage := s.makeStorage(c, controller)
	result, err := storage.URL("grasshopper.avi")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, "heavy lies")
	checkCalls(c, controller.Stub, makeCall("GetFile", "prefix-grasshopper.avi"))
}

func (s *maasStorageSuite) TestPut(c *tc.C) {
	controller := newFakeControllerWithErrors(errors.New("oh no!"))
	storage := s.makeStorage(c, controller)
	reader := bytes.NewReader([]byte{})
	err := storage.Put("riff", reader, 10)
	c.Assert(err, tc.ErrorMatches, "oh no!")
	checkCalls(c, controller.Stub, makeCall("AddFile", gomaasapi.AddFileArgs{
		Filename: "prefix-riff",
		Reader:   reader,
		Length:   10,
	}))
}

func (s *maasStorageSuite) TestRemoveNoSuchFile(c *tc.C) {
	controller := newFakeControllerWithErrors(errors.New("oh no!"))
	storage := s.makeStorage(c, controller)
	err := storage.Remove("FIOS")
	c.Assert(err, tc.ErrorMatches, "oh no!")
}

func (s *maasStorageSuite) TestRemoveErrorFromDelete(c *tc.C) {
	controller := newFakeControllerWithFiles(
		&fakeFile{name: "prefix-FIOS", error: errors.New("protected")},
	)
	storage := s.makeStorage(c, controller)
	err := storage.Remove("FIOS")
	c.Assert(err, tc.ErrorMatches, "protected")
	checkCalls(c, controller.Stub, makeCall("GetFile", "prefix-FIOS"))
}

func (s *maasStorageSuite) TestRemoveAll(c *tc.C) {
	controller := newFakeControllerWithFiles(
		&fakeFile{name: "prefix-zack"},
		&fakeFile{name: "prefix-kevin", error: errors.New("oops")},
		&fakeFile{name: "prefix-jim"},
		&fakeFile{name: "prefix-riff"},
	)
	storage := s.makeStorage(c, controller)
	err := storage.RemoveAll()
	c.Assert(err, tc.ErrorMatches, "cannot delete all provider state: oops")
	controller.Stub.CheckCall(c, 0, "Files", "prefix-")

	deleteds := make([]bool, 4)
	for i, file := range controller.files {
		deleteds[i] = file.(*fakeFile).deleted
	}
	c.Assert(deleteds, tc.DeepEquals, []bool{true, true, true, true})
}
