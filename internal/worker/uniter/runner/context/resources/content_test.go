// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"io"
	"strings"

	charmresource "github.com/juju/juju/charm/resource"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/context/resources"
)

var _ = gc.Suite(&ContentSuite{})

type ContentSuite struct {
	testing.IsolationSuite
	stub *testing.Stub
}

func (s *ContentSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
}

func (s *ContentSuite) TestVerifyOkay(c *gc.C) {
	info, reader := newResource(c, s.stub, "spam", "some data")
	content := resources.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}

	err := content.Verify(info.Size, info.Fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckNoCalls(c)
}

func (s *ContentSuite) TestVerifyBadSize(c *gc.C) {
	info, reader := newResource(c, s.stub, "spam", "some data")
	content := resources.Content{
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
	info, reader := newResource(c, s.stub, "spam", "some data")
	content := resources.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}

	err = content.Verify(info.Size, fp)

	c.Check(err, gc.ErrorMatches, `resource fingerprint does not match expected .*`)
	s.stub.CheckNoCalls(c)
}

var _ = gc.Suite(&CheckerSuite{})

type CheckerSuite struct {
	testing.IsolationSuite
	stub *testing.Stub
}

func (s *CheckerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
}

func (s *CheckerSuite) TestVerifyOkay(c *gc.C) {
	info, reader := newResource(c, s.stub, "spam", "some data")
	checker := resources.NewContentChecker(
		resources.Content{
			Size:        info.Size,
			Fingerprint: info.Fingerprint,
		},
	)
	wrapped := checker.WrapReader(reader)

	s.stub.CheckNoCalls(c)
	data, err := io.ReadAll(wrapped)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(data), gc.Equals, "some data")
	err = checker.Verify()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CheckerSuite) TestVerifyFailed(c *gc.C) {
	info, reader := newResource(c, s.stub, "spam", "some data")
	checker := resources.NewContentChecker(
		resources.Content{
			Size:        info.Size + 1,
			Fingerprint: info.Fingerprint,
		},
	)
	wrapped := checker.WrapReader(reader)

	s.stub.CheckNoCalls(c)
	_, err := io.ReadAll(wrapped)
	c.Assert(err, jc.ErrorIsNil)
	err = checker.Verify()
	c.Assert(err, gc.ErrorMatches, "resource size does not match expected \\(9 != 10\\)")
}
