// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/context/internal"
	"github.com/juju/juju/resource/resourcetesting"
)

var _ = gc.Suite(&DirectorySpecSuite{})

type DirectorySpecSuite struct {
	testing.IsolationSuite

	stub *internalStub
}

func (s *DirectorySpecSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = newInternalStub()
}

func (s *DirectorySpecSuite) TestNewDirectorySpec(c *gc.C) {
	dataDir := "/var/lib/juju/agents/unit-spam-1/resources"
	deps := s.stub

	spec := internal.NewDirectorySpec(dataDir, "eggs", deps)

	s.stub.CheckCallNames(c, "Join")
	c.Check(spec, jc.DeepEquals, &internal.DirectorySpec{
		Name:    "eggs",
		Dirname: dataDir + "/eggs",
		Deps:    deps,
	})
}

func (s *DirectorySpecSuite) TestResolveFile(c *gc.C) {
	dataDir := "/var/lib/juju/agents/unit-spam-1/resources"
	deps := s.stub
	spec := internal.NewDirectorySpec(dataDir, "eggs", deps)
	s.stub.ResetCalls()

	resolved := spec.Resolve("ham/ham.tgz")

	s.stub.CheckCallNames(c, "Join")
	c.Check(resolved, gc.Equals, dataDir+"/eggs/ham/ham.tgz")
}

func (s *DirectorySpecSuite) TestResolveEmpty(c *gc.C) {
	dataDir := "/var/lib/juju/agents/unit-spam-1/resources"
	deps := s.stub
	spec := internal.NewDirectorySpec(dataDir, "eggs", deps)
	s.stub.ResetCalls()

	resolved := spec.Resolve()

	s.stub.CheckCallNames(c, "Join")
	c.Check(resolved, gc.Equals, dataDir+"/eggs")
}

func (s *DirectorySpecSuite) TestIsUpToDateTrue(c *gc.C) {
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	content := internal.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}
	s.stub.ReturnFingerprintMatches = true
	dataDir := "/var/lib/juju/agents/unit-spam-1/resources"
	deps := s.stub
	spec := internal.NewDirectorySpec(dataDir, "eggs", deps)
	s.stub.ResetCalls()

	isUpToDate, err := spec.IsUpToDate(content)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Join", "FingerprintMatches")
	c.Check(isUpToDate, jc.IsTrue)
}

func (s *DirectorySpecSuite) TestIsUpToDateFalse(c *gc.C) {
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	content := internal.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}
	s.stub.ReturnFingerprintMatches = false
	dataDir := "/var/lib/juju/agents/unit-spam-1/resources"
	deps := s.stub
	spec := internal.NewDirectorySpec(dataDir, "eggs", deps)
	s.stub.ResetCalls()

	isUpToDate, err := spec.IsUpToDate(content)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Join", "FingerprintMatches")
	c.Check(isUpToDate, jc.IsFalse)
}

func (s *DirectorySpecSuite) TestIsUpToDateCalls(c *gc.C) {
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	content := internal.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}
	dataDir := "/var/lib/juju/agents/unit-spam-1/resources"
	deps := s.stub
	spec := internal.NewDirectorySpec(dataDir, "eggs", deps)
	s.stub.ResetCalls()

	_, err := spec.IsUpToDate(content)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Join", "FingerprintMatches")
	dirname := s.stub.Join(dataDir, "eggs")
	s.stub.CheckCall(c, 0, "Join", []string{dirname, "eggs"})
	s.stub.CheckCall(c, 1, "FingerprintMatches", s.stub.Join(dirname, "eggs"), info.Fingerprint)
}

func (s *DirectorySpecSuite) TestInitialize(c *gc.C) {
	dataDir := "/var/lib/juju/agents/unit-spam-1/resources"
	deps := s.stub
	spec := internal.NewDirectorySpec(dataDir, "eggs", deps)
	s.stub.ResetCalls()

	dir, err := spec.Initialize()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "MkdirAll")
	s.stub.CheckCall(c, 0, "MkdirAll", spec.Dirname)
	c.Check(dir, jc.DeepEquals, &internal.Directory{
		DirectorySpec: spec,
		Deps:          deps,
	})
}

var _ = gc.Suite(&TempDirectorySpecSuite{})

type TempDirectorySpecSuite struct {
	testing.IsolationSuite

	stub *internalStub
}

func (s *TempDirectorySpecSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = newInternalStub()
}

var _ = gc.Suite(&DirectorySuite{})

type DirectorySuite struct {
	testing.IsolationSuite

	stub *internalStub
}

func (s *DirectorySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = newInternalStub()
}

func (s *DirectorySuite) TestNewDirectory(c *gc.C) {
	dataDir := "/var/lib/juju/agents/unit-spam-1/resources"
	deps := s.stub
	spec := internal.NewDirectorySpec(dataDir, "eggs", deps)
	s.stub.ResetCalls()

	dir := internal.NewDirectory(spec, deps)

	s.stub.CheckNoCalls(c)
	c.Check(dir, jc.DeepEquals, &internal.Directory{
		DirectorySpec: spec,
		Deps:          deps,
	})
}

func (s *DirectorySuite) TestWrite(c *gc.C) {
	res := resourcetesting.NewResource(c, s.stub.Stub, "spam", "a-application", "some data")
	stub := &stubDirectory{
		internalStub: s.stub,
	}
	stub.ReturnInfo = res.Resource
	opened := stub
	dataDir := "/var/lib/juju/agents/unit-spam-1/resources"
	deps := s.stub
	spec := internal.NewDirectorySpec(dataDir, "eggs", deps)
	s.stub.ResetCalls()
	dir := internal.NewDirectory(spec, deps)

	err := dir.Write(opened)
	c.Assert(err, jc.ErrorIsNil)

	stub.CheckCallNames(c,
		"Info",
		"Content",
		"Join",
		"CreateWriter",
		"WriteContent",
		"CloseAndLog",
	)
}

func (s *DirectorySuite) TestWriteContent(c *gc.C) {
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	content := internal.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}
	relPath := info.Path
	stub := &stubDirectory{
		internalStub: s.stub,
	}
	dataDir := "/var/lib/juju/agents/unit-spam-1/resources"
	deps := s.stub
	spec := internal.NewDirectorySpec(dataDir, "eggs", deps)
	dir := internal.NewDirectory(spec, deps)
	s.stub.ResetCalls()

	err := dir.WriteContent(relPath, content)
	c.Assert(err, jc.ErrorIsNil)

	stub.CheckCallNames(c,
		"Join",
		"CreateWriter",
		"WriteContent",
		"CloseAndLog",
	)
}

type stubDirectory struct {
	*internalStub

	ReturnInfo    resource.Resource
	ReturnContent internal.Content
}

func (s *stubDirectory) Info() resource.Resource {
	s.AddCall("Info")
	s.NextErr() // Pop one off.

	return s.ReturnInfo
}

func (s *stubDirectory) Content() internal.Content {
	s.AddCall("Content")
	s.NextErr() // Pop one off.

	return s.ReturnContent
}
