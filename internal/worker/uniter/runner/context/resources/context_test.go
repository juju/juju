// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"os"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/context/mocks"
	"github.com/juju/juju/internal/worker/uniter/runner/context/resources"
	coretesting "github.com/juju/juju/testing"
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

	client.EXPECT().GetResource(gomock.Any(), "spam").Return(info, reader, nil)

	ctx := resources.ResourcesHookContext{
		Client:       client,
		ResourcesDir: resourceDir,
		Logger:       coretesting.NoopLogger{},
	}
	path, err := ctx.DownloadResource(context.Background(), "spam")
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

	client.EXPECT().GetResource(gomock.Any(), "spam").Return(info, reader, nil)

	ctx := resources.ResourcesHookContext{
		Client:       client,
		ResourcesDir: resourceDir,
		Logger:       coretesting.NoopLogger{},
	}
	path, err := ctx.DownloadResource(context.Background(), "spam")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Close")
	c.Assert(path, gc.Equals, filepath.Join(resourceDir, "spam.tgz"))
}
