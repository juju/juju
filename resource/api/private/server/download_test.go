// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api/private/server"
	"github.com/juju/juju/resource/resourcetesting"
)

var _ = gc.Suite(&DownloadSuite{})

type DownloadSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
	deps *stubDownloadDeps
}

func (s *DownloadSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.deps = &stubDownloadDeps{
		Stub: s.stub,
	}
}

func (s *DownloadSuite) TestHandleDownloadOkay(c *gc.C) {
	expected := resourcetesting.NewResource(c, s.stub, "spam", "a-service", "some data")
	s.deps.ReturnOpenResource = expected

	opened, err := server.HandleDownload("spam", s.deps)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"OpenResource",
	)
	c.Check(opened, jc.DeepEquals, expected)
}

type stubDownloadDeps struct {
	*testing.Stub

	ReturnOpenResource resource.Opened
}

func (s *stubDownloadDeps) OpenResource(name string) (resource.Opened, error) {
	s.AddCall("OpenResource", name)
	if err := s.NextErr(); err != nil {
		return resource.Opened{}, errors.Trace(err)
	}

	return s.ReturnOpenResource, nil
}
