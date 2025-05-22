// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"io"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/runner/context/resources"
)

func TestContentSuite(t *stdtesting.T) {
	tc.Run(t, &ContentSuite{})
}

type ContentSuite struct {
	testhelpers.IsolationSuite
	stub *testhelpers.Stub
}

func (s *ContentSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub = &testhelpers.Stub{}
}

func (s *ContentSuite) TestVerifyOkay(c *tc.C) {
	info, reader := newResource(c, s.stub, "spam", "some data")
	content := resources.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}

	err := content.Verify(info.Size, info.Fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	s.stub.CheckNoCalls(c)
}

func (s *ContentSuite) TestVerifyBadSize(c *tc.C) {
	info, reader := newResource(c, s.stub, "spam", "some data")
	content := resources.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}

	err := content.Verify(info.Size+1, info.Fingerprint)

	c.Check(err, tc.ErrorMatches, `resource size does not match expected \(10 != 9\)`)
	s.stub.CheckNoCalls(c)
}

func (s *ContentSuite) TestVerifyBadFingerprint(c *tc.C) {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader("other data"))
	c.Assert(err, tc.ErrorIsNil)
	info, reader := newResource(c, s.stub, "spam", "some data")
	content := resources.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	}

	err = content.Verify(info.Size, fp)

	c.Check(err, tc.ErrorMatches, `resource fingerprint does not match expected .*`)
	s.stub.CheckNoCalls(c)
}
func TestCheckerSuite(t *stdtesting.T) {
	tc.Run(t, &CheckerSuite{})
}

type CheckerSuite struct {
	testhelpers.IsolationSuite
	stub *testhelpers.Stub
}

func (s *CheckerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub = &testhelpers.Stub{}
}

func (s *CheckerSuite) TestVerifyOkay(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "some data")
	err = checker.Verify()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CheckerSuite) TestVerifyFailed(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	err = checker.Verify()
	c.Assert(err, tc.ErrorMatches, "resource size does not match expected \\(9 != 10\\)")
}
