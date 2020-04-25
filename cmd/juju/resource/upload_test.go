// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"bytes"

	charmresource "github.com/juju/charm/v7/resource"
	jujucmd "github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	resourcecmd "github.com/juju/juju/cmd/juju/resource"
	"github.com/juju/juju/resource"
)

var _ = gc.Suite(&UploadSuite{})

type UploadSuite struct {
	testing.IsolationSuite

	stub     *testing.Stub
	stubDeps *stubUploadDeps
}

func (s *UploadSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.stubDeps = &stubUploadDeps{
		stub:   s.stub,
		client: &stubAPIClient{stub: s.stub},
	}
}

func (*UploadSuite) TestInitEmpty(c *gc.C) {
	var u resourcecmd.UploadCommand

	err := u.Init([]string{})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (*UploadSuite) TestInitOneArg(c *gc.C) {
	u := resourcecmd.NewUploadCommandForTest(resourcecmd.UploadDeps{})
	err := u.Init([]string{"foo"})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (*UploadSuite) TestInitJustName(c *gc.C) {
	u := resourcecmd.NewUploadCommandForTest(resourcecmd.UploadDeps{})

	err := u.Init([]string{"foo", "bar"})
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (*UploadSuite) TestInitNoName(c *gc.C) {
	u := resourcecmd.NewUploadCommandForTest(resourcecmd.UploadDeps{})

	err := u.Init([]string{"foo", "=foobar"})
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (*UploadSuite) TestInitNoPath(c *gc.C) {
	u := resourcecmd.NewUploadCommandForTest(resourcecmd.UploadDeps{})

	err := u.Init([]string{"foo", "foobar="})
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (*UploadSuite) TestInitGood(c *gc.C) {
	u := resourcecmd.NewUploadCommandForTest(resourcecmd.UploadDeps{})

	err := u.Init([]string{"foo", "bar=baz"})
	c.Assert(err, jc.ErrorIsNil)
	svc, name, filename := resourcecmd.UploadCommandResourceValue(u)
	c.Assert(svc, gc.Equals, "foo")
	c.Assert(name, gc.Equals, "bar")
	c.Assert(filename, gc.Equals, "baz")
	c.Assert(resourcecmd.UploadCommandApplication(u), gc.Equals, "foo")
}

func (*UploadSuite) TestInitTwoResources(c *gc.C) {
	u := resourcecmd.NewUploadCommandForTest(resourcecmd.UploadDeps{})

	err := u.Init([]string{"foo", "bar=baz", "fizz=buzz"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["fizz=buzz"\]`)
}

func (s *UploadSuite) TestInfo(c *gc.C) {
	var command resourcecmd.UploadCommand
	info := command.Info()

	c.Check(info, jc.DeepEquals, &jujucmd.Info{
		Name:    "attach-resource",
		Args:    "application name=file|OCI image",
		Purpose: "Update a resource for an application.",
		Doc: `
This command updates a resource for an application.

For file resources, it uploads a file from your local disk to the juju controller to be
streamed to the charm when "resource-get" is called by a hook.

For OCI image resources used by k8s applications, an OCI image or file path is specified.
A file is specified when a private OCI image is needed and the username/password used to
access the image is needed along with the image path.
`,
		Aliases:        []string{"attach"},
		FlagKnownAs:    "option",
		ShowSuperFlags: []string{"show-log", "debug", "logging-config", "verbose", "quiet", "h", "help"},
	})
}

func (s *UploadSuite) TestUploadFileResource(c *gc.C) {
	file := &stubFile{stub: s.stub}
	s.stubDeps.file = file
	u := resourcecmd.NewUploadCommandForTest(resourcecmd.UploadDeps{
		NewClient:    s.stubDeps.NewClient,
		OpenResource: s.stubDeps.OpenResource,
	},
	)
	err := u.Init([]string{"svc", "foo=bar"})
	c.Assert(err, jc.ErrorIsNil)

	err = u.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

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
	s.stub.CheckCall(c, 3, "Upload", "svc", "foo", "bar", file)
}

func (s *UploadSuite) TestUploadFileChangeBlocked(c *gc.C) {
	file := &stubFile{stub: s.stub}
	s.stubDeps.file = file
	u := resourcecmd.NewUploadCommandForTest(resourcecmd.UploadDeps{
		NewClient:    s.stubDeps.NewClient,
		OpenResource: s.stubDeps.OpenResource,
	},
	)
	err := u.Init([]string{"svc", "foo=bar"})
	c.Assert(err, jc.ErrorIsNil)

	expectedError := params.Error{
		Message: "test-block",
		Code:    params.CodeOperationBlocked,
	}
	s.stub.SetErrors(nil, nil, nil, expectedError)

	err = u.Run(nil)
	c.Assert(err.Error(), jc.Contains, `failed to upload resource "foo": test-block`)
	c.Assert(err.Error(), jc.Contains, `All operations that change model have been disabled for the current model.`)

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
	s.stub.CheckCall(c, 3, "Upload", "svc", "foo", "bar", file)
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

func (s *UploadSuite) TestUploadDockerResource(c *gc.C) {
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
	u := resourcecmd.NewUploadCommandForTest(resourcecmd.UploadDeps{
		NewClient:    s.stubDeps.NewClient,
		OpenResource: s.stubDeps.OpenResource,
	},
	)
	err := u.Init([]string{"svc", "foo=bar"})
	c.Assert(err, jc.ErrorIsNil)

	err = u.Run(nil)
	c.Assert(err, jc.ErrorIsNil)

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
	stub   *testing.Stub
	file   resourcecmd.ReadSeekCloser
	client resourcecmd.UploadClient
}

func (s *stubUploadDeps) NewClient(c *resourcecmd.UploadCommand) (resourcecmd.UploadClient, error) {
	s.stub.AddCall("NewClient", c)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.client, nil
}

func (s *stubUploadDeps) OpenResource(path string) (resourcecmd.ReadSeekCloser, error) {
	s.stub.AddCall("OpenResource", path)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.file, nil
}
