// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package model_test

import (
	"context"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/model"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	_ "github.com/juju/juju/internal/provider/ec2" // needed when getting valid local credentials
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&ModelCredentialCommandSuite{})

type ModelCredentialCommandSuite struct {
	jujutesting.IsolationSuite

	store *jujuclient.MemStore

	modelClient fakeModelClient
	cloudClient fakeCloudClient
	rootFunc    func(ctx context.Context) (base.APICallCloser, error)
}

func (s *ModelCredentialCommandSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User: "admin",
	}
	err := s.store.UpdateModel("testing", "admin/mymodel", jujuclient.ModelDetails{
		ModelUUID: testing.ModelTag.Id(),
		ModelType: coremodel.IAAS,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.store.Models["testing"].CurrentModel = "admin/mymodel"

	s.rootFunc = func(ctx context.Context) (base.APICallCloser, error) { return &fakeRoot{}, nil }
	s.modelClient = fakeModelClient{}
	s.cloudClient = fakeCloudClient{}
}

func (s *ModelCredentialCommandSuite) TestBadArguments(c *tc.C) {
	badArgs := []struct {
		about  string
		args   []string
		err    string
		stderr string
	}{{
		about:  "no arguments",
		args:   []string{},
		err:    regexp.QuoteMeta("Usage: juju set-credential [options] <cloud name> <credential name>"),
		stderr: "ERROR Usage: juju set-credential [options] <cloud name> <credential name>\n",
	}, {
		about:  "1 argument",
		args:   []string{"cloud"},
		err:    regexp.QuoteMeta("Usage: juju set-credential [options] <cloud name> <credential name>"),
		stderr: "ERROR Usage: juju set-credential [options] <cloud name> <credential name>\n",
	}, {
		about:  "3 argument",
		args:   []string{"cloud", "cred", "nothing"},
		err:    regexp.QuoteMeta("Usage: juju set-credential [options] <cloud name> <credential name>"),
		stderr: "ERROR Usage: juju set-credential [options] <cloud name> <credential name>\n",
	}, {
		about:  "not valid cloud name",
		args:   []string{"#1foo", "cred"},
		err:    "cloud name \"#1foo\" not valid",
		stderr: "ERROR cloud name \"#1foo\" not valid\n",
	}, {
		about:  "not valid cloud credential name",
		args:   []string{"cloud", "0foo"},
		err:    "cloud credential name \"0foo\" not valid",
		stderr: "ERROR cloud credential name \"0foo\" not valid\n",
	}}

	for i, bad := range badArgs {
		c.Logf("%d: %v", i, bad.about)
		ctx, err := cmdtesting.RunCommand(c, s.newSetCredentialCommand(), bad.args...)
		c.Assert(err, tc.ErrorMatches, bad.err)

		c.Assert(cmdtesting.Stderr(ctx), tc.Equals, bad.stderr)
		c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")

		s.modelClient.CheckNoCalls(c)
		s.cloudClient.CheckNoCalls(c)
	}
}

func (s *ModelCredentialCommandSuite) TestRootAPIError(c *tc.C) {
	s.rootFunc = func(ctx context.Context) (base.APICallCloser, error) {
		return nil, errors.New("kaboom")
	}
	ctx, err := cmdtesting.RunCommand(c, s.newSetCredentialCommand(), "cloud", "credential")
	c.Assert(err, tc.ErrorMatches, "opening API connection: kaboom")

	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "Failed to change model credential: opening API connection: kaboom\n")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")

	s.modelClient.CheckNoCalls(c)
	s.cloudClient.CheckNoCalls(c)
}

func (s *ModelCredentialCommandSuite) TestSetCredentialNotFoundAnywhere(c *tc.C) {
	s.assertCredentialNotFound(c, `
Did not find credential remotely. Looking locally...
Failed to change model credential: loading credentials: credentials for cloud aws not found
`[1:])
}

func (s *ModelCredentialCommandSuite) TestSetCredentialRemoteSearchErred(c *tc.C) {
	s.cloudClient.SetErrors(errors.New("boom"))
	s.assertCredentialNotFound(c, `
Could not determine if there are remote credentials for the user: boom
Did not find credential remotely. Looking locally...
Failed to change model credential: loading credentials: credentials for cloud aws not found
`[1:])
}

func (s *ModelCredentialCommandSuite) assertCredentialNotFound(c *tc.C, expectedStderr string) {
	ctx, err := cmdtesting.RunCommand(c, s.newSetCredentialCommand(), "aws", "credential")
	c.Assert(err, tc.ErrorMatches, "loading credentials: credentials for cloud aws not found")

	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, expectedStderr)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")

	s.modelClient.CheckNoCalls(c)
	s.cloudClient.CheckCalls(c, []jujutesting.StubCall{
		{"UserCredentials", []interface{}{
			names.NewUserTag("admin"),
			names.NewCloudTag("aws"),
		}},
		{"Close", nil},
	})
}

func (s *ModelCredentialCommandSuite) TestSetCredentialFoundRemote(c *tc.C) {
	err := s.assertRemoteCredentialFound(c, `
Found credential remotely, on the controller. Not looking locally...
Changed cloud credential on model "admin/mymodel" to "credential".
`[1:])
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ModelCredentialCommandSuite) TestSetCredentialErred(c *tc.C) {
	s.modelClient.SetErrors(errors.New("kaboom"))
	err := s.assertRemoteCredentialFound(c, `
Found credential remotely, on the controller. Not looking locally...
`[1:])
	c.Assert(err, tc.ErrorMatches, "could not set model credential: kaboom")
}

func (s *ModelCredentialCommandSuite) TestSetCredentialBlocked(c *tc.C) {
	s.modelClient.SetErrors(&params.Error{Code: params.CodeOperationBlocked, Message: "nope"})
	err := s.assertRemoteCredentialFound(c, `
Found credential remotely, on the controller. Not looking locally...
`[1:])
	c.Assert(err.Error(), tc.Contains, `could not set model credential: nope`)
	c.Assert(err.Error(), tc.Contains, `All operations that change model have been disabled for the current model.`)
}

func (s *ModelCredentialCommandSuite) assertRemoteCredentialFound(c *tc.C, expectedStderr string) error {
	credentialTag := names.NewCloudCredentialTag("aws/admin/credential")
	s.cloudClient.userCredentials = []names.CloudCredentialTag{credentialTag}
	ctx, err := cmdtesting.RunCommand(c, s.newSetCredentialCommand(), "aws", "credential")

	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, expectedStderr)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")

	s.modelClient.CheckCalls(c, []jujutesting.StubCall{
		{"ChangeModelCredential", []interface{}{
			testing.ModelTag,
			credentialTag,
		}},
		{"Close", nil},
	})
	s.cloudClient.CheckCalls(c, []jujutesting.StubCall{
		{"UserCredentials", []interface{}{
			names.NewUserTag("admin"),
			names.NewCloudTag("aws"),
		}},
		{"Close", nil},
	})
	// This the error from running the command.
	// It's returned to allow individual test to assert their expectations.
	return err
}

func (s *ModelCredentialCommandSuite) TestSetCredentialLocal(c *tc.C) {
	err := s.assertLocalCredentialUsed(c, `
Did not find credential remotely. Looking locally...
Uploading local credential to the controller.
Changed cloud credential on model "admin/mymodel" to "credential".
`[1:])
	c.Assert(err, tc.ErrorIsNil)

	s.modelClient.CheckCalls(c, []jujutesting.StubCall{
		{"ChangeModelCredential", []interface{}{
			testing.ModelTag,
			names.NewCloudCredentialTag("aws/admin/credential"),
		}},
		{"Close", nil},
	})
}

func (s *ModelCredentialCommandSuite) TestSetCredentialLocalUploadFailed(c *tc.C) {
	s.cloudClient.SetErrors(nil, errors.New("upload failed"))
	err := s.assertLocalCredentialUsed(c, `
Did not find credential remotely. Looking locally...
Uploading local credential to the controller.
Failed to change model credential: upload failed
`[1:])
	c.Assert(err, tc.ErrorMatches, "upload failed")
	s.modelClient.CheckNoCalls(c)
}

func (s *ModelCredentialCommandSuite) assertLocalCredentialUsed(c *tc.C, expectedStderr string) error {
	credential := cloud.NewCredential(cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": "v",
			"secret-key": "v",
		},
	)
	cloudCredential := &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"credential": credential,
		},
	}
	s.store.Credentials["aws"] = *cloudCredential
	ctx, err := cmdtesting.RunCommand(c, s.newSetCredentialCommand(), "aws", "credential")

	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, expectedStderr)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")

	s.cloudClient.CheckCalls(c, []jujutesting.StubCall{
		{"UserCredentials", []interface{}{
			names.NewUserTag("admin"),
			names.NewCloudTag("aws"),
		}},
		{"AddCredential", []interface{}{
			names.NewCloudCredentialTag("aws/admin/credential").String(),
			credential,
		}},
		{"Close", nil},
	})
	return err
}

func (s *ModelCredentialCommandSuite) newSetCredentialCommand() cmd.Command {
	return model.NewModelCredentialCommandForTest(&s.modelClient, &s.cloudClient, s.rootFunc, s.store)
}

type fakeModelClient struct {
	jujutesting.Stub
}

func (f *fakeModelClient) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeModelClient) ChangeModelCredential(ctx context.Context, model names.ModelTag, credential names.CloudCredentialTag) error {
	f.MethodCall(f, "ChangeModelCredential", model, credential)
	return f.NextErr()
}

type fakeCloudClient struct {
	jujutesting.Stub

	userCredentials []names.CloudCredentialTag
}

func (f *fakeCloudClient) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeCloudClient) UserCredentials(ctx context.Context, u names.UserTag, c names.CloudTag) ([]names.CloudCredentialTag, error) {
	f.MethodCall(f, "UserCredentials", u, c)
	return f.userCredentials, f.NextErr()
}

func (f *fakeCloudClient) AddCredential(ctx context.Context, tag string, credential cloud.Credential) error {
	f.MethodCall(f, "AddCredential", tag, credential)
	return f.NextErr()
}

type fakeRoot struct {
	base.APICaller
	jujutesting.Stub
}

func (f *fakeRoot) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}
