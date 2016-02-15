// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"io"
	"io/ioutil"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource/context/internal"
)

var _ = gc.Suite(&ContentSuite{})

type ContentSuite struct {
	testing.IsolationSuite

	stub *internalStub
}

func (s *ContentSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = newInternalStub()
}

func (s *ContentSuite) TestVerifyOkay(c *gc.C) {
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	content := internal.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}

	err := content.Verify(info.Size, info.Fingerprint)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
}

func (s *ContentSuite) TestVerifyBadSize(c *gc.C) {
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	content := internal.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}

	err := content.Verify(info.Size+1, info.Fingerprint)

	c.Check(err, gc.ErrorMatches, `resource size does not match expected \(10 != 9\)`)
	s.stub.CheckNoCalls(c)
}

func (s *ContentSuite) TestVerifyBadFingerprint(c *gc.C) {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader("other data"))
	c.Assert(err, jc.ErrorIsNil)
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	content := internal.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}

	err = content.Verify(info.Size, fp)

	c.Check(err, gc.ErrorMatches, `resource fingerprint does not match expected .*`)
	s.stub.CheckNoCalls(c)
}

func (s *ContentSuite) TestWriteContent(c *gc.C) {
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	content := internal.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}
	target, _ := filetesting.NewStubWriter(s.stub.Stub)
	stub := &stubContent{
		internalStub: s.stub,
		Reader:       reader,
	}
	stub.ReturnNewChecker = stub
	stub.ReturnWrapReader = stub
	deps := stub

	err := internal.WriteContent(target, content, deps)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"NewChecker",
		"WrapReader",
		"Copy",
		"Verify",
	)
}

var _ = gc.Suite(&CheckerSuite{})

type CheckerSuite struct {
	testing.IsolationSuite

	stub *internalStub
}

func (s *CheckerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = newInternalStub()
}

func (s *CheckerSuite) TestNewContentChecker(c *gc.C) {
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	content := internal.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}
	sizeWriter, sizeBuf := filetesting.NewStubWriter(s.stub.Stub)
	sizeTracker := &stubChecker{
		Writer: sizeWriter,
		stub:   s.stub.Stub,
	}
	sumWriter, sumBuf := filetesting.NewStubWriter(s.stub.Stub)
	checksumWriter := &stubChecker{
		Writer: sumWriter,
		stub:   s.stub.Stub,
	}

	checker := internal.NewContentChecker(content, sizeTracker, checksumWriter)

	s.stub.CheckNoCalls(c)
	c.Check(checker, jc.DeepEquals, &internal.Checker{
		Content:        content,
		SizeTracker:    sizeTracker,
		ChecksumWriter: checksumWriter,
	})
	c.Check(sizeBuf.String(), gc.Equals, "")
	c.Check(sumBuf.String(), gc.Equals, "")
}

func (s *CheckerSuite) TestWrapReader(c *gc.C) {
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	sizeWriter, sizeBuf := filetesting.NewStubWriter(s.stub.Stub)
	sumWriter, sumBuf := filetesting.NewStubWriter(s.stub.Stub)
	checker := internal.Checker{
		Content: internal.Content{
			Size:        info.Size,
			Fingerprint: info.Fingerprint,
		},
		SizeTracker: &stubChecker{
			Writer: sizeWriter,
			stub:   s.stub.Stub,
		},
		ChecksumWriter: &stubChecker{
			Writer: sumWriter,
			stub:   s.stub.Stub,
		},
	}

	wrapped := checker.WrapReader(reader)

	s.stub.CheckNoCalls(c)
	data, err := ioutil.ReadAll(wrapped)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c,
		"Read",
		"Write",
		"Write",
		"Read",
	)
	c.Check(string(data), gc.Equals, "some data")
	c.Check(sizeBuf.String(), gc.Equals, "some data")
	c.Check(sumBuf.String(), gc.Equals, "some data")
}

func (s *CheckerSuite) TestVerifyOkay(c *gc.C) {
	info, _ := newResource(c, s.stub.Stub, "spam", "some data")
	stub := &stubChecker{
		stub:              s.stub.Stub,
		ReturnSize:        info.Size,
		ReturnFingerprint: info.Fingerprint,
	}
	checker := internal.Checker{
		Content: internal.Content{
			Size:        info.Size,
			Fingerprint: info.Fingerprint,
		},
		SizeTracker:    stub,
		ChecksumWriter: stub,
	}

	err := checker.Verify()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Size", "Fingerprint")
}

func (s *CheckerSuite) TestVerifyFailed(c *gc.C) {
	info, _ := newResource(c, s.stub.Stub, "spam", "some data")
	stub := &stubChecker{
		stub:              s.stub.Stub,
		ReturnSize:        info.Size + 1,
		ReturnFingerprint: info.Fingerprint,
	}
	checker := internal.Checker{
		Content: internal.Content{
			Size:        info.Size,
			Fingerprint: info.Fingerprint,
		},
		SizeTracker:    stub,
		ChecksumWriter: stub,
	}

	err := checker.Verify()

	s.stub.CheckCallNames(c, "Size", "Fingerprint")
	c.Check(err, gc.ErrorMatches, `resource size does not match expected \(10 != 9\)`)
}

func (s *CheckerSuite) TestNopWrapReader(c *gc.C) {
	_, reader := newResource(c, s.stub.Stub, "spam", "some data")
	checker := internal.NopChecker{}

	wrapped := checker.WrapReader(reader)

	s.stub.CheckNoCalls(c)
	c.Check(wrapped, gc.Equals, reader)
}

func (s *CheckerSuite) TestNopVerify(c *gc.C) {
	checker := internal.NopChecker{}

	err := checker.Verify()

	c.Check(err, jc.ErrorIsNil)
}

type stubContent struct {
	*internalStub
	io.Reader

	ReturnWrapReader   io.Reader
	ReturnCreateTarget io.WriteCloser
}

func (s *stubContent) WrapReader(reader io.Reader) io.Reader {
	s.Stub.AddCall("WrapReader", reader)
	s.Stub.NextErr() // Pop one off.

	return s.ReturnWrapReader
}

func (s *stubContent) Verify() error {
	s.Stub.AddCall("Verify")
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type stubChecker struct {
	io.Writer
	stub *testing.Stub

	ReturnSize        int64
	ReturnFingerprint charmresource.Fingerprint
}

func (s *stubChecker) Size() int64 {
	s.stub.AddCall("Size")
	s.stub.NextErr() // Pop one off.

	return s.ReturnSize
}

func (s *stubChecker) Fingerprint() charmresource.Fingerprint {
	s.stub.AddCall("Fingerprint")
	s.stub.NextErr() // Pop one off.

	return s.ReturnFingerprint
}
