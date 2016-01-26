// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"io"
	"path"

	"github.com/juju/errors"
	"github.com/juju/testing"
	"github.com/juju/testing/filetesting"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/context/internal"
)

type internalStub struct {
	*testing.Stub

	ReturnGetResourceInfo         resource.Resource
	ReturnGetResourceData         io.ReadCloser
	ReturnNewContextDirectorySpec internal.ContextDirectorySpec
	ReturnOpenResource            internal.ContextOpenedResource
	ReturnNewTempDirSpec          internal.DownloadTempTarget
	ReturnNewChecker              internal.ContentChecker
	ReturnCreateTarget            io.WriteCloser
	ReturnCreateFile              io.WriteCloser
	ReturnNewTempDir              string
}

func newInternalStub() *internalStub {
	stub := &testing.Stub{}
	return &internalStub{
		Stub: stub,
	}
}

func (s *internalStub) GetResource(name string) (resource.Resource, io.ReadCloser, error) {
	s.Stub.AddCall("GetResource", name)
	if err := s.Stub.NextErr(); err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return s.ReturnGetResourceInfo, s.ReturnGetResourceData, nil
}

func (s *internalStub) NewContextDirectorySpec() internal.ContextDirectorySpec {
	s.Stub.AddCall("NewContextDirectorySpec")
	s.Stub.NextErr() // Pop one off.

	return s.ReturnNewContextDirectorySpec
}

func (s *internalStub) OpenResource() (internal.ContextOpenedResource, error) {
	s.Stub.AddCall("OpenResource")
	if err := s.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnOpenResource, nil
}

func (s *internalStub) Download(spec internal.Resolver, remote internal.ContextOpenedResource) error {
	s.Stub.AddCall("Download", spec, remote)
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *internalStub) ReplaceDirectory(tgt, src string) error {
	s.Stub.AddCall("ReplaceDirectory", tgt, src)
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *internalStub) NewTempDirSpec() (internal.DownloadTempTarget, error) {
	s.Stub.AddCall("NewTempDirSpec")
	if err := s.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnNewTempDirSpec, nil
}

func (s *internalStub) NewChecker(content internal.Content) internal.ContentChecker {
	s.Stub.AddCall("NewChecker", content)
	s.Stub.NextErr() // Pop one off.

	return s.ReturnNewChecker
}

func (s *internalStub) WriteContent(filename string, content internal.Content) error {
	s.Stub.AddCall("WriteContent", filename, content)
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *internalStub) CloseAndLog(closer io.Closer, label string) {
	s.Stub.AddCall("CloseAndLog", closer, label)
	s.Stub.NextErr() // Pop one off.
}

func (s *internalStub) MkdirAll(dirname string) error {
	s.Stub.AddCall("MkdirAll", dirname)
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *internalStub) CreateTarget() (io.WriteCloser, error) {
	s.Stub.AddCall("CreateTarget")
	if err := s.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnCreateTarget, nil
}

func (s *internalStub) CreateFile(filename string) (io.WriteCloser, error) {
	s.Stub.AddCall("CreateFile", filename)
	if err := s.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnCreateFile, nil
}

func (s *internalStub) NewTempDir() (string, error) {
	s.Stub.AddCall("NewTempDir")
	if err := s.Stub.NextErr(); err != nil {
		return "", errors.Trace(err)
	}

	return s.ReturnNewTempDir, nil
}

func (s *internalStub) RemoveDir(dirname string) error {
	s.Stub.AddCall("RemoveDir", dirname)
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *internalStub) Move(target, source string) error {
	s.Stub.AddCall("Move", target, source)
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *internalStub) Join(pth ...string) string {
	s.Stub.AddCall("Join", pth)
	s.Stub.NextErr() // Pop one off.

	return path.Join(pth...)
}

type stubReadCloser struct {
	io.Reader
	io.Closer
}

func newStubReadCloser(stub *testing.Stub, content string) io.ReadCloser {
	return &stubReadCloser{
		Reader: filetesting.NewStubReader(stub, content),
		Closer: &filetesting.StubCloser{
			Stub: stub,
		},
	}
}
