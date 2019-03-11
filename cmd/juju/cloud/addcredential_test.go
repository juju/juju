// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/environs"
	environsTesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/testing"
)

type addCredentialSuite struct {
	testing.BaseSuite

	store           *jujuclient.MemStore
	schema          map[jujucloud.AuthType]jujucloud.CredentialSchema
	authTypes       []jujucloud.AuthType
	cloudByNameFunc func(string) (*jujucloud.Cloud, error)
}

var _ = gc.Suite(&addCredentialSuite{
	store: jujuclient.NewMemStore(),
})

func (s *addCredentialSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	unreg := environs.RegisterProvider("mock-addcredential-provider", &mockProvider{credSchemas: &s.schema})
	s.AddCleanup(func(_ *gc.C) {
		unreg()
	})
	s.cloudByNameFunc = func(cloud string) (*jujucloud.Cloud, error) {
		if cloud != "somecloud" && cloud != "anothercloud" {
			return nil, errors.NotFoundf("cloud %v", cloud)
		}
		return &jujucloud.Cloud{
			Type:             "mock-addcredential-provider",
			AuthTypes:        s.authTypes,
			Endpoint:         "cloud-endpoint",
			IdentityEndpoint: "cloud-identity-endpoint",
		}, nil
	}
}

func (s *addCredentialSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.store.Credentials = make(map[string]jujucloud.CloudCredential)
}

func (s *addCredentialSuite) run(c *gc.C, stdin io.Reader, args ...string) (*cmd.Context, error) {
	addCmd := cloud.NewAddCredentialCommandForTest(s.store, s.cloudByNameFunc)
	err := cmdtesting.InitCommand(addCmd, args)
	if err != nil {
		return nil, err
	}
	ctx := cmdtesting.Context(c)
	ctx.Stdin = stdin
	return ctx, addCmd.Run(ctx)
}

func (s *addCredentialSuite) TestBadArgs(c *gc.C) {
	_, err := s.run(c, nil)
	c.Assert(err, gc.ErrorMatches, `Usage: juju add-credential <cloud-name> \[-f <credentials.yaml>\]`)
	_, err = s.run(c, nil, "somecloud", "-f", "credential.yaml", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *addCredentialSuite) TestBadCloudName(c *gc.C) {
	_, err := s.run(c, nil, "badcloud")
	c.Assert(err, gc.ErrorMatches, "cloud badcloud not valid")
}

func (s *addCredentialSuite) TestAddFromFileBadFilename(c *gc.C) {
	_, err := s.run(c, nil, "somecloud", "-f", "somefile.yaml")
	c.Assert(err, gc.ErrorMatches, ".*open somefile.yaml: .*")
}

func (s *addCredentialSuite) TestNoCredentialsRequired(c *gc.C) {
	s.authTypes = nil
	_, err := s.run(c, nil, "somecloud")
	c.Assert(err, gc.ErrorMatches, `cloud "somecloud" does not require credentials`)
}

func (s *addCredentialSuite) createTestCredentialData(c *gc.C) string {
	return s.createTestCredentialDataWithAuthType(c, "access-key")
}

func (s *addCredentialSuite) createTestCredentialDataWithAuthType(c *gc.C, authType string) string {
	dir := c.MkDir()
	credsFile := filepath.Join(dir, "cred.yaml")
	data := fmt.Sprintf(`
credentials:
  somecloud:
    me:
      auth-type: %v
      access-key: <key>
      secret-key: <secret>
`[1:], authType)
	err := ioutil.WriteFile(credsFile, []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)
	return credsFile
}

func (s *addCredentialSuite) TestAddFromFileWithInvalidCredentialNames(c *gc.C) {
	dir := c.MkDir()
	sourceFile := filepath.Join(dir, "cred.yaml")
	err := ioutil.WriteFile(sourceFile, []byte(`
credentials:
  somecloud:
    credential with spaces:
      auth-type: interactive
      trust-password: "123"
`), 0644)
	c.Assert(err, gc.IsNil)

	s.authTypes = []jujucloud.AuthType{jujucloud.InteractiveAuthType}
	_, err = s.run(c, nil, "somecloud", "-f", sourceFile)
	c.Assert(err, gc.ErrorMatches, `"credential with spaces" is not a valid credential name`)
}

func (s *addCredentialSuite) TestAddFromFileNoCredentialsFound(c *gc.C) {
	sourceFile := s.createTestCredentialData(c)
	_, err := s.run(c, nil, "anothercloud", "-f", sourceFile)
	c.Assert(err, gc.ErrorMatches, `no credentials for cloud anothercloud exist in file.*`)
}

func (s *addCredentialSuite) TestAddFromFileExisting(c *gc.C) {
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{"cred": {}},
		},
	}
	sourceFile := s.createTestCredentialData(c)
	_, err := s.run(c, nil, "somecloud", "-f", sourceFile)
	c.Assert(err, gc.ErrorMatches, `local credentials for cloud "somecloud" already exist; use --replace to overwrite / merge`)
}

func (s *addCredentialSuite) TestAddFromFileExistingReplace(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType, jujucloud.AccessKeyAuthType}
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"cred": jujucloud.NewCredential(jujucloud.UserPassAuthType, nil)},
		},
	}
	sourceFile := s.createTestCredentialData(c)
	_, err := s.run(c, nil, "somecloud", "-f", sourceFile, "--replace")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.store.Credentials, jc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"cred": jujucloud.NewCredential(jujucloud.UserPassAuthType, nil),
				"me": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{
					"access-key": "<key>",
					"secret-key": "<secret>",
				})},
		},
	})
}

func (s *addCredentialSuite) TestAddNewFromFile(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.AccessKeyAuthType}
	sourceFile := s.createTestCredentialData(c)
	_, err := s.run(c, nil, "somecloud", "-f", sourceFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.store.Credentials, jc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"me": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{
					"access-key": "<key>",
					"secret-key": "<secret>",
				})},
		},
	})
}

func (s *addCredentialSuite) TestAddInvalidAuth(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.AccessKeyAuthType}
	sourceFile := s.createTestCredentialDataWithAuthType(c, "invalid auth")
	_, err := s.run(c, nil, "somecloud", "-f", sourceFile)
	c.Assert(err, gc.ErrorMatches,
		regexp.QuoteMeta(`credential "me" contains invalid auth type "invalid auth", valid auth types for cloud "somecloud" are [access-key]`))
}

func (s *addCredentialSuite) TestAddCloudUnsupportedAuth(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.AccessKeyAuthType}
	sourceFile := s.createTestCredentialDataWithAuthType(c, fmt.Sprintf("%v", jujucloud.JSONFileAuthType))
	_, err := s.run(c, nil, "somecloud", "-f", sourceFile)
	c.Assert(err, gc.ErrorMatches,
		regexp.QuoteMeta(`credential "me" contains invalid auth type "jsonfile", valid auth types for cloud "somecloud" are [access-key]`))
}

func (s *addCredentialSuite) assertAddUserpassCredential(c *gc.C, input string, expected *jujucloud.Credential, msg string) {
	s.schema = map[jujucloud.AuthType]jujucloud.CredentialSchema{
		jujucloud.UserPassAuthType: {
			{
				"username", jujucloud.CredentialAttr{Optional: false},
			}, {
				"password", jujucloud.CredentialAttr{Hidden: true},
			},
		},
	}
	stdin := strings.NewReader(input)
	ctx, err := s.run(c, stdin, "somecloud")
	c.Assert(err, jc.ErrorIsNil)
	var cred jujucloud.Credential
	if expected == nil {
		cred = jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{
			"username": "user",
			"password": "password",
		})
	} else {
		cred = *expected
	}
	c.Assert(s.store.Credentials, jc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"fred": cred,
			},
		},
	})
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, msg)
}

func (s *addCredentialSuite) TestAddCredentialSingleAuthType(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType}
	expected := `
Enter credential name: 
Using auth-type "userpass".

Enter username: 
Enter password: 
Credential "fred" added locally for cloud "somecloud".

`[1:]
	s.assertAddUserpassCredential(c, "fred\nuser\npassword\n", nil, expected)
}

func (s *addCredentialSuite) TestAddCredentialRetryOnMissingMandatoryAttribute(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType}
	expected := `
Enter credential name: 
Using auth-type "userpass".

Enter username: 
Enter username: 
Enter password: 
Credential "fred" added locally for cloud "somecloud".

`[1:]
	s.assertAddUserpassCredential(c, "fred\n\nuser\npassword\n", nil, expected)
}

func (s *addCredentialSuite) TestAddCredentialMultipleAuthType(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType, jujucloud.AccessKeyAuthType}
	expected := `
Enter credential name: 
Auth Types
  userpass
  access-key

Select auth type [userpass]: 
Enter username: 
Enter password: 
Credential "fred" added locally for cloud "somecloud".

`[1:]
	s.assertAddUserpassCredential(c, "fred\nuserpass\nuser\npassword\n", nil, expected)
}

func (s *addCredentialSuite) TestAddCredentialInteractive(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{"interactive"}
	s.schema = map[jujucloud.AuthType]jujucloud.CredentialSchema{
		"interactive": {{"username", jujucloud.CredentialAttr{}}},
	}

	stdin := strings.NewReader("bobscreds\nbob\n")
	ctx, err := s.run(c, stdin, "somecloud")
	c.Assert(err, jc.ErrorIsNil)

	// there's an extra line return after Using auth-type because the rest get a
	// second line return from the user hitting return when they enter a value
	// (which is not shown here), but that one does not.
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Enter credential name: 
Using auth-type "interactive".

Enter username: 
Credential "bobscreds" added locally for cloud "somecloud".

`[1:])

	// FinalizeCredential should have generated a userpass credential
	// based on the input from the interactive credential.
	c.Assert(s.store.Credentials, jc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"bobscreds": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{
					"username":             "bob",
					"password":             "cloud-endpoint",
					"application-password": "cloud-identity-endpoint",
				}),
			},
		},
	})
}

func (s *addCredentialSuite) TestAddInvalidCredentialInteractive(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{"interactive"}
	s.schema = map[jujucloud.AuthType]jujucloud.CredentialSchema{
		"interactive": {{"username", jujucloud.CredentialAttr{}}},
	}

	stdin := strings.NewReader("credential name with spaces\n")
	ctx, err := s.run(c, stdin, "somecloud")
	c.Assert(err, gc.NotNil)

	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Enter credential name: Invalid credential name: "credential name with spaces"

Enter credential name: 
`[1:])
}

func (s *addCredentialSuite) TestAddCredentialCredSchemaInteractive(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType}
	s.schema = map[jujucloud.AuthType]jujucloud.CredentialSchema{
		"interactive": {{"username", jujucloud.CredentialAttr{}}},
		jujucloud.UserPassAuthType: {
			{
				"username", jujucloud.CredentialAttr{Optional: false},
			}, {
				"password", jujucloud.CredentialAttr{Hidden: true},
			},
		},
	}

	stdin := strings.NewReader("bobscreds\n\nbob\n")
	ctx, err := s.run(c, stdin, "somecloud")
	c.Assert(err, jc.ErrorIsNil)

	// there's an extra line return after Using auth-type because the rest get a
	// second line return from the user hitting return when they enter a value
	// (which is not shown here), but that one does not.
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Enter credential name: 
Auth Types
  userpass
  interactive

Select auth type [interactive]: 
Enter username: 
Credential "bobscreds" added locally for cloud "somecloud".

`[1:])

	// FinalizeCredential should have generated a userpass credential
	// based on the input from the interactive credential.
	c.Assert(s.store.Credentials, jc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"bobscreds": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{
					"username":             "bob",
					"password":             "cloud-endpoint",
					"application-password": "cloud-identity-endpoint",
				}),
			},
		},
	})
}

func (s *addCredentialSuite) TestAddCredentialReplace(c *gc.C) {
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"fred": jujucloud.NewCredential(jujucloud.UserPassAuthType, nil)},
		},
	}
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType}
	expected := `
Enter credential name: 
A credential "fred" already exists locally on this client.
Replace local credential? (y/N): 
Using auth-type "userpass".

Enter username: 
Enter password: 
Credential "fred" updated locally for cloud "somecloud".

`[1:]
	s.assertAddUserpassCredential(c, "fred\ny\nuser\npassword\n", nil, expected)
}

func (s *addCredentialSuite) TestAddCredentialReplaceDecline(c *gc.C) {
	cred := jujucloud.NewCredential(jujucloud.UserPassAuthType, nil)
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"fred": cred},
		},
	}
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType}
	expected := `
Enter credential name: 
A credential "fred" already exists locally on this client.
Replace local credential? (y/N): 
`[1:]
	s.assertAddUserpassCredential(c, "fred\nn\n", &cred, expected)
}

func (s *addCredentialSuite) assertAddFileCredential(c *gc.C, input, fileKey string) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "jsonfile")
	err := ioutil.WriteFile(filename, []byte{}, 0600)
	c.Assert(err, jc.ErrorIsNil)

	stdin := strings.NewReader(fmt.Sprintf(input, filename))
	addCmd := cloud.NewAddCredentialCommandForTest(s.store, s.cloudByNameFunc)
	err = cmdtesting.InitCommand(addCmd, []string{"somecloud"})
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.ContextForDir(c, dir)
	ctx.Stdin = stdin
	err = addCmd.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.store.Credentials, jc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"fred": jujucloud.NewCredential(s.authTypes[0], map[string]string{
					fileKey: filename,
				}),
			},
		},
	})
}

func (s *addCredentialSuite) TestAddJsonFileCredential(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.JSONFileAuthType}
	s.schema = map[jujucloud.AuthType]jujucloud.CredentialSchema{
		jujucloud.JSONFileAuthType: {
			{
				"file",
				jujucloud.CredentialAttr{
					Optional: false,
					FilePath: true,
				},
			},
		},
	}
	// Input includes invalid file info.
	s.assertAddFileCredential(c, "fred\nbadfile\n.\n%s\n", "file")
}

func (s *addCredentialSuite) TestAddCredentialWithFileAttr(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType}
	s.schema = map[jujucloud.AuthType]jujucloud.CredentialSchema{
		jujucloud.UserPassAuthType: {
			{
				"key",
				jujucloud.CredentialAttr{
					FileAttr: "key-file",
				},
			},
		},
	}
	// Input includes invalid file info.
	s.assertAddFileCredential(c, "fred\nbadfile\n.\n%s\n", "key-file")
}

func (s *addCredentialSuite) assertAddCredentialWithOptions(c *gc.C, input string) {
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType}
	s.schema = map[jujucloud.AuthType]jujucloud.CredentialSchema{
		jujucloud.UserPassAuthType: {
			{
				"username", jujucloud.CredentialAttr{Optional: false},
			}, {
				"algorithm", jujucloud.CredentialAttr{Options: []interface{}{"optionA", "optionB"}},
			},
		},
	}
	// Input includes a bad option
	stdin := strings.NewReader(input)
	_, err := s.run(c, stdin, "somecloud")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.store.Credentials, jc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"fred": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{
					"username":  "user",
					"algorithm": "optionA",
				}),
			},
		},
	})
}

func (s *addCredentialSuite) TestAddCredentialWithOptions(c *gc.C) {
	s.assertAddCredentialWithOptions(c, "fred\nuser\nbadoption\noptionA\n")
}

func (s *addCredentialSuite) TestAddCredentialWithOptionsAutofill(c *gc.C) {
	s.assertAddCredentialWithOptions(c, "fred\nuser\n\n")
}

func (s *addCredentialSuite) TestAddMAASCredential(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.OAuth1AuthType}
	s.schema = map[jujucloud.AuthType]jujucloud.CredentialSchema{
		jujucloud.OAuth1AuthType: {
			{
				"maas-oauth", jujucloud.CredentialAttr{},
			},
		},
	}
	stdin := strings.NewReader("fred\nauth:token\n")
	_, err := s.run(c, stdin, "somecloud")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.store.Credentials, jc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"fred": jujucloud.NewCredential(jujucloud.OAuth1AuthType, map[string]string{
					"maas-oauth": "auth:token",
				}),
			},
		},
	})
}

func (s *addCredentialSuite) TestAddGCEFileCredentials(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.JSONFileAuthType}
	s.schema = map[jujucloud.AuthType]jujucloud.CredentialSchema{
		jujucloud.JSONFileAuthType: {
			{
				"file",
				jujucloud.CredentialAttr{
					Description: "path to the credential file",
					Optional:    false,
					FilePath:    true,
				},
			},
		},
	}
	sourceFile := s.createTestCredentialDataWithAuthType(c, fmt.Sprintf("%v", jujucloud.JSONFileAuthType))
	stdin := strings.NewReader(fmt.Sprintf("blah\n%s\n", sourceFile))
	ctx, err := s.run(c, stdin, "somecloud")
	c.Assert(err, jc.ErrorIsNil)
	expected := `
Enter credential name: 
Using auth-type "jsonfile".

Enter path to the credential file: 
Credential "blah" added locally for cloud "somecloud".

`[1:]
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected)
}

func (s *addCredentialSuite) TestShouldFinalizeCredentialWithEnvironProvider(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	provider := environsTesting.NewMockEnvironProvider(ctrl)
	cred := jujucloud.Credential{}
	got := cloud.ShouldFinalizeCredential(provider, cred)
	c.Assert(got, jc.IsFalse)
}

func (s *addCredentialSuite) TestShouldFinalizeCredentialSuccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	provider := struct {
		environs.EnvironProvider
		*environsTesting.MockRequestFinalizeCredential
	}{
		EnvironProvider:               environsTesting.NewMockEnvironProvider(ctrl),
		MockRequestFinalizeCredential: environsTesting.NewMockRequestFinalizeCredential(ctrl),
	}

	cred := jujucloud.Credential{}
	provider.MockRequestFinalizeCredential.EXPECT().ShouldFinalizeCredential(cred).Return(true)

	got := cloud.ShouldFinalizeCredential(provider, cred)
	c.Assert(got, jc.IsTrue)
}

func (s *addCredentialSuite) TestShouldFinalizeCredentialFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	provider := struct {
		environs.EnvironProvider
		*environsTesting.MockRequestFinalizeCredential
	}{
		EnvironProvider:               environsTesting.NewMockEnvironProvider(ctrl),
		MockRequestFinalizeCredential: environsTesting.NewMockRequestFinalizeCredential(ctrl),
	}

	cred := jujucloud.Credential{}
	provider.MockRequestFinalizeCredential.EXPECT().ShouldFinalizeCredential(cred).Return(false)

	got := cloud.ShouldFinalizeCredential(provider, cred)
	c.Assert(got, jc.IsFalse)
}
