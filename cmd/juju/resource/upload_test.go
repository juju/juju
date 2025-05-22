// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"bytes"
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	resourcecmd "github.com/juju/juju/cmd/juju/resource"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

func TestUploadSuite(t *stdtesting.T) {
	tc.Run(t, &UploadSuite{})
}

type UploadSuite struct {
	testhelpers.IsolationSuite

	stub     *testhelpers.Stub
	stubDeps *stubUploadDeps
}

func (s *UploadSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testhelpers.Stub{}
	s.stubDeps = &stubUploadDeps{
		stub:   s.stub,
		client: &stubAPIClient{stub: s.stub},
	}
}

func (*UploadSuite) TestInitEmpty(c *tc.C) {
	var u resourcecmd.UploadCommand

	err := u.Init([]string{})
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
}

func (s *UploadSuite) TestInitOneArg(c *tc.C) {
	u := resourcecmd.NewUploadCommandForTest(nil, s.stubDeps)
	err := u.Init([]string{"foo"})
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
}

func (s *UploadSuite) TestInitJustName(c *tc.C) {
	u := resourcecmd.NewUploadCommandForTest(nil, s.stubDeps)

	err := u.Init([]string{"foo", "bar"})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *UploadSuite) TestInitNoName(c *tc.C) {
	u := resourcecmd.NewUploadCommandForTest(nil, s.stubDeps)

	err := u.Init([]string{"foo", "=foobar"})
	c.Assert(errors.Cause(err), tc.ErrorIs, errors.NotValid)
}

func (s *UploadSuite) TestInitNoPath(c *tc.C) {
	u := resourcecmd.NewUploadCommandForTest(nil, s.stubDeps)

	err := u.Init([]string{"foo", "foobar="})
	c.Assert(errors.Cause(err), tc.ErrorIs, errors.NotValid)
}

func (s *UploadSuite) TestInitGood(c *tc.C) {
	u := resourcecmd.NewUploadCommandForTest(nil, s.stubDeps)

	err := u.Init([]string{"foo", "bar=baz"})
	c.Assert(err, tc.ErrorIsNil)
	svc, name, filename := resourcecmd.UploadCommandResourceValue(u)
	c.Assert(svc, tc.Equals, "foo")
	c.Assert(name, tc.Equals, "bar")
	c.Assert(filename, tc.Equals, "baz")
	c.Assert(resourcecmd.UploadCommandApplication(u), tc.Equals, "foo")
}

func (s *UploadSuite) TestInitTwoResources(c *tc.C) {
	u := resourcecmd.NewUploadCommandForTest(nil, s.stubDeps)

	err := u.Init([]string{"foo", "bar=baz", "fizz=buzz"})
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["fizz=buzz"\]`)
}

func (s *UploadSuite) TestInfo(c *tc.C) {
	var command resourcecmd.UploadCommand
	info := command.Info()

	// Verify that Info is wired up. Without verifying exact text.
	c.Check(info.Name, tc.Equals, "attach-resource")
	c.Check(info.Purpose, tc.Not(tc.Equals), "")
	c.Check(info.Doc, tc.Not(tc.Equals), "")
	c.Check(info.FlagKnownAs, tc.Not(tc.Equals), "")
	c.Check(len(info.ShowSuperFlags), tc.GreaterThan, 2)
}

func (s *UploadSuite) TestUploadFileResource(c *tc.C) {
	file := &stubFile{stub: s.stub}
	s.stubDeps.file = file
	u := resourcecmd.NewUploadCommandForTest(s.stubDeps.NewClient, s.stubDeps)
	err := u.Init([]string{"svc", "foo=bar"})
	c.Assert(err, tc.ErrorIsNil)

	err = u.Run(nil)
	c.Assert(err, tc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"NewClient",
		"ListResources",
		"OpenResource",
		"Upload",
		"FileClose",
		"Close",
	)
	s.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
	s.stub.CheckCall(c, 2, "OpenResource", "bar")
	s.stub.CheckCall(c, 3, "Upload", "svc", "foo", "bar", "", file)
}

func (s *UploadSuite) TestUploadFileChangeBlocked(c *tc.C) {
	file := &stubFile{stub: s.stub}
	s.stubDeps.file = file
	u := resourcecmd.NewUploadCommandForTest(s.stubDeps.NewClient, s.stubDeps)
	err := u.Init([]string{"svc", "foo=bar"})
	c.Assert(err, tc.ErrorIsNil)

	expectedError := params.Error{
		Message: "test-block",
		Code:    params.CodeOperationBlocked,
	}
	s.stub.SetErrors(nil, nil, nil, expectedError)

	err = u.Run(nil)
	c.Assert(err.Error(), tc.Contains, `failed to upload resource "foo": test-block`)
	c.Assert(err.Error(), tc.Contains, `All operations that change model have been disabled for the current model.`)

	s.stub.CheckCallNames(c,
		"NewClient",
		"ListResources",
		"OpenResource",
		"Upload",
		"FileClose",
		"Close",
	)
	s.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
	s.stub.CheckCall(c, 2, "OpenResource", "bar")
	s.stub.CheckCall(c, 3, "Upload", "svc", "foo", "bar", "", file)
}

type rsc struct {
	*bytes.Buffer
}

func (r rsc) Close() error {
	return nil
}
func (rsc) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}

func (s *UploadSuite) TestUploadDockerResource(c *tc.C) {
	fileContents := `
registrypath: registry.staging.jujucharms.com/wallyworld/mysql-k8s/mysql_image
username: docker-registry
password: hunter2
`
	s.stubDeps.file = rsc{bytes.NewBuffer([]byte(fileContents))}
	s.stubDeps.client.(*stubAPIClient).resources = resource.ApplicationResources{
		Resources: []resource.Resource{{Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "foo",
				Type: charmresource.TypeContainerImage,
			},
		}}},
	}
	u := resourcecmd.NewUploadCommandForTest(s.stubDeps.NewClient, s.stubDeps)
	err := u.Init([]string{"svc", "foo=bar"})
	c.Assert(err, tc.ErrorIsNil)

	err = u.Run(nil)
	c.Assert(err, tc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"NewClient",
		"ListResources",
		"OpenResource",
		"Upload",
		"Close",
	)
	s.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
	s.stub.CheckCall(c, 2, "OpenResource", "bar")
}

type stubUploadDeps struct {
	modelcmd.Filesystem
	stub   *testhelpers.Stub
	file   modelcmd.ReadSeekCloser
	client resourcecmd.UploadClient
}

func (s *stubUploadDeps) NewClient(ctx context.Context) (resourcecmd.UploadClient, error) {
	s.stub.AddCall("NewClient")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.client, nil
}

func (s *stubUploadDeps) Open(path string) (modelcmd.ReadSeekCloser, error) {
	s.stub.AddCall("OpenResource", path)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.file, nil
}
