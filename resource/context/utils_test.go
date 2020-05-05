// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"bytes"
	"io"
	"os"
	"strings"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource/context"
)

var _ = gc.Suite(&UtilsSuite{})

type UtilsSuite struct {
	testing.IsolationSuite

	stub    *testing.Stub
	matcher *stubMatcher
}

func (s *UtilsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.matcher = &stubMatcher{stub: s.stub}
}

func (s *UtilsSuite) newReader(c *gc.C, content string) (io.ReadCloser, charmresource.Fingerprint) {
	r := filetesting.NewStubFile(s.stub, bytes.NewBufferString(content))

	tmpReader := strings.NewReader(content)
	fp, err := charmresource.GenerateFingerprint(tmpReader)
	c.Assert(err, jc.ErrorIsNil)

	return r, fp
}

func (s *UtilsSuite) TestFingerprintMatchesOkay(c *gc.C) {
	r, expected := s.newReader(c, "spam")
	s.matcher.ReturnOpen = r
	s.matcher.ReturnGenerateFingerprint = expected
	matcher := context.FingerprintMatcher{
		Open:                s.matcher.Open,
		GenerateFingerprint: s.matcher.GenerateFingerprint,
	}

	matches, err := matcher.FingerprintMatches("some/filename.txt", expected)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(matches, jc.IsTrue)
}

func (s *UtilsSuite) TestFingerprintMatchesDepCalls(c *gc.C) {
	r, expected := s.newReader(c, "spam")
	s.matcher.ReturnOpen = r
	s.matcher.ReturnGenerateFingerprint = expected
	matcher := context.FingerprintMatcher{
		Open:                s.matcher.Open,
		GenerateFingerprint: s.matcher.GenerateFingerprint,
	}

	matches, err := matcher.FingerprintMatches("some/filename.txt", expected)
	c.Assert(matches, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Open", "GenerateFingerprint", "Close")
	s.stub.CheckCall(c, 0, "Open", "some/filename.txt")
	s.stub.CheckCall(c, 1, "GenerateFingerprint", r)
}

func (s *UtilsSuite) TestFingerprintMatchesNotFound(c *gc.C) {
	_, expected := s.newReader(c, "spam")
	matcher := context.FingerprintMatcher{
		Open: s.matcher.Open,
	}
	s.stub.SetErrors(os.ErrNotExist)

	matches, err := matcher.FingerprintMatches("some/filename.txt", expected)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(matches, jc.IsFalse)
	s.stub.CheckCallNames(c, "Open")
}

func (s *UtilsSuite) TestFingerprintMatchesOpenFailed(c *gc.C) {
	_, expected := s.newReader(c, "spam")
	matcher := context.FingerprintMatcher{
		Open: s.matcher.Open,
	}
	failure := errors.New("<failed>")
	s.stub.SetErrors(failure)

	_, err := matcher.FingerprintMatches("some/filename.txt", expected)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "Open")
}

func (s *UtilsSuite) TestFingerprintMatchesGenerateFingerprintFailed(c *gc.C) {
	r, expected := s.newReader(c, "spam")
	s.matcher.ReturnOpen = r
	matcher := context.FingerprintMatcher{
		Open:                s.matcher.Open,
		GenerateFingerprint: s.matcher.GenerateFingerprint,
	}
	failure := errors.New("<failed>")
	s.stub.SetErrors(nil, failure)

	_, err := matcher.FingerprintMatches("some/filename.txt", expected)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "Open", "GenerateFingerprint", "Close")
}

type stubMatcher struct {
	stub *testing.Stub

	ReturnOpen                io.ReadCloser
	ReturnGenerateFingerprint charmresource.Fingerprint
}

func (s *stubMatcher) Open(filename string) (io.ReadCloser, error) {
	s.stub.AddCall("Open", filename)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnOpen, nil
}

func (s *stubMatcher) GenerateFingerprint(r io.Reader) (charmresource.Fingerprint, error) {
	s.stub.AddCall("GenerateFingerprint", r)
	if err := s.stub.NextErr(); err != nil {
		return charmresource.Fingerprint{}, errors.Trace(err)
	}

	return s.ReturnGenerateFingerprint, nil
}
