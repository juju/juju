// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"os"
	"path/filepath"

	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/uniter/runner/context/mocks"
	"github.com/juju/juju/internal/worker/uniter/runner/context/resources"
)

var _ = tc.Suite(&ContextSuite{})

type ContextSuite struct {
	testing.IsolationSuite
	stub *testing.Stub
}

func (s *ContextSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
}

func (s *ContextSuite) TestDownloadOutOfDate(c *tc.C) {
	info, reader := newResource(c, s.stub, "spam", "some data")
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	resourceDir := c.MkDir()
	client := mocks.NewMockOpenedResourceClient(ctrl)

	client.EXPECT().GetResource(gomock.Any(), "spam").Return(info, reader, nil)

	ctx := resources.ResourcesHookContext{
		Client:       client,
		ResourcesDir: resourceDir,
		Logger:       loggertesting.WrapCheckLog(c),
	}
	path, err := ctx.DownloadResource(context.Background(), "spam")
	c.Assert(err, tc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Read", "Read", "Close")
	c.Assert(path, tc.Equals, filepath.Join(resourceDir, "spam.tgz"))
	data, err := os.ReadFile(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, "some data")
}

func (s *ContextSuite) TestContextDownloadUpToDate(c *tc.C) {
	info, reader := newResource(c, s.stub, "spam", "some data")
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	resourceDir := c.MkDir()
	existing := filepath.Join(resourceDir, "spam.tgz")
	err := os.WriteFile(existing, []byte("some data"), 0755)
	c.Assert(err, tc.ErrorIsNil)

	client := mocks.NewMockOpenedResourceClient(ctrl)

	client.EXPECT().GetResource(gomock.Any(), "spam").Return(info, reader, nil)

	ctx := resources.ResourcesHookContext{
		Client:       client,
		ResourcesDir: resourceDir,
		Logger:       loggertesting.WrapCheckLog(c),
	}
	path, err := ctx.DownloadResource(context.Background(), "spam")
	c.Assert(err, tc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Close")
	c.Assert(path, tc.Equals, filepath.Join(resourceDir, "spam.tgz"))
}
