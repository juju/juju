// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/context/internal"
)

var _ = gc.Suite(&ContextSuite{})

type ContextSuite struct {
	testing.IsolationSuite

	stub *internalStub
}

func (s *ContextSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = newInternalStub()
}

func (s *ContextSuite) TestContextDownloadOutOfDate(c *gc.C) {
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	content := internal.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}
	stub := &stubContext{
		internalStub: s.stub,
		StubCloser:   &filetesting.StubCloser{Stub: s.stub.Stub},
	}
	stub.ReturnNewContextDirectorySpec = stub
	stub.ReturnOpenResource = stub
	stub.ReturnResolve = "/var/lib/juju/agents/unit-spam-1/resources/spam/eggs.tgz"
	stub.ReturnInfo = info
	stub.ReturnContent = content
	deps := stub

	path, err := internal.ContextDownload(deps)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"NewContextDirectorySpec",
		"OpenResource",
		"Info",
		"Resolve",
		"Content",
		"IsUpToDate",
		"Download",
		"CloseAndLog",
	)
	c.Check(path, gc.Equals, "/var/lib/juju/agents/unit-spam-1/resources/spam/eggs.tgz")
}

func (s *ContextSuite) TestContextDownloadUpToDate(c *gc.C) {
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	content := internal.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}
	stub := &stubContext{
		internalStub: s.stub,
		StubCloser:   &filetesting.StubCloser{Stub: s.stub.Stub},
	}
	stub.ReturnNewContextDirectorySpec = stub
	stub.ReturnOpenResource = stub
	stub.ReturnResolve = "/var/lib/juju/agents/unit-spam-1/resources/spam/eggs.tgz"
	stub.ReturnInfo = info
	stub.ReturnContent = content
	stub.ReturnIsUpToDate = true
	deps := stub

	path, err := internal.ContextDownload(deps)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"NewContextDirectorySpec",
		"OpenResource",
		"Info",
		"Resolve",
		"Content",
		"IsUpToDate",
		"CloseAndLog",
	)
	c.Check(path, gc.Equals, "/var/lib/juju/agents/unit-spam-1/resources/spam/eggs.tgz")
}

type stubContext struct {
	*internalStub
	*filetesting.StubCloser

	ReturnResolve    string
	ReturnInfo       resource.Resource
	ReturnContent    internal.Content
	ReturnInitialize internal.DownloadDirectory
	ReturnIsUpToDate bool
}

func (s *stubContext) Resolve(path ...string) string {
	s.AddCall("Resolve", path)
	s.NextErr() // Pop one off.

	return s.ReturnResolve
}

func (s *stubContext) Info() resource.Resource {
	s.AddCall("Info")
	s.NextErr() // Pop one off.

	return s.ReturnInfo
}

func (s *stubContext) Content() internal.Content {
	s.AddCall("Content")
	s.NextErr() // Pop one off.

	return s.ReturnContent
}

func (s *stubContext) Initialize() (internal.DownloadDirectory, error) {
	s.AddCall("Initialize")
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnInitialize, nil
}

func (s *stubContext) IsUpToDate(content internal.Content) (bool, error) {
	s.AddCall("IsUpToDate", content)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	return s.ReturnIsUpToDate, nil
}
