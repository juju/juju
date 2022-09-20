// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"os"
	"path/filepath"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/context/mocks"
	"github.com/juju/juju/worker/uniter/runner/context/resources"
)

var _ = gc.Suite(&ContextSuite{})

type ContextSuite struct {
	testing.IsolationSuite
	stub *testing.Stub
}

func (s *ContextSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
}

func (s *ContextSuite) TestDownloadOutOfDate(c *gc.C) {
	info, reader := newResource(c, s.stub, "spam", "some data")
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	resourceDir := c.MkDir()
	client := mocks.NewMockOpenedResourceClient(ctrl)

	client.EXPECT().GetResource("spam").Return(info, reader, nil)

	ctx := resources.ResourcesHookContext{
		Client:       client,
		ResourcesDir: resourceDir,
		Logger:       coretesting.NoopLogger{},
	}
	path, err := ctx.DownloadResource("spam")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Read", "Read", "Close")
	c.Assert(path, gc.Equals, filepath.Join(resourceDir, "spam.tgz"))
	data, err := os.ReadFile(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "some data")
}

func (s *ContextSuite) TestContextDownloadUpToDate(c *gc.C) {
	info, reader := newResource(c, s.stub, "spam", "some data")
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	resourceDir := c.MkDir()
	existing := filepath.Join(resourceDir, "spam.tgz")
	err := os.WriteFile(existing, []byte("some data"), 0755)
	c.Assert(err, jc.ErrorIsNil)

	client := mocks.NewMockOpenedResourceClient(ctrl)

	client.EXPECT().GetResource("spam").Return(info, reader, nil)

	ctx := resources.ResourcesHookContext{
		Client:       client,
		ResourcesDir: resourceDir,
		Logger:       coretesting.NoopLogger{},
	}
	path, err := ctx.DownloadResource("spam")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Close")
	c.Assert(path, gc.Equals, filepath.Join(resourceDir, "spam.tgz"))
}
