// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource/context/internal"
)

var _ = gc.Suite(&DownloadSuite{})

type DownloadSuite struct {
	testing.IsolationSuite

	stub *internalStub
}

func (s *DownloadSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = newInternalStub()
}

func (s *DownloadSuite) TestDownload(c *gc.C) {
	stub := &stubDownload{
		internalStub: s.stub,
	}
	target := stub
	remote := stub

	err := internal.Download(target, remote)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Initialize", "Write")
	s.stub.CheckCall(c, 1, "Write", remote)
}

type stubDownload struct {
	*internalStub
	internal.ContentSource

	ReturnResolve []string
}

func (s *stubDownload) Close() error {
	s.Stub.AddCall("Close")
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubDownload) Initialize() (internal.DownloadDirectory, error) {
	s.Stub.AddCall("Initialize")
	if err := s.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s, nil
}

func (s *stubDownload) Write(source internal.ContentSource) error {
	s.Stub.AddCall("Write", source)
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubDownload) Resolve(path ...string) string {
	s.Stub.AddCall("Resolve", path)
	_ = s.Stub.NextErr() // Pop one off.

	resolved := s.ReturnResolve[0]
	s.ReturnResolve = s.ReturnResolve[1:]
	return resolved
}
