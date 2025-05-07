// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/environs"
	environsTesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	_ "github.com/juju/juju/internal/provider/dummy"
	_ "github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type addCredentialSuite struct {
	testing.BaseSuite

	store             *jujuclient.MemStore
	schema            map[jujucloud.AuthType]jujucloud.CredentialSchema
	authTypes         []jujucloud.AuthType
	cloudByNameFunc   func(string) (*jujucloud.Cloud, error)
	credentialAPIFunc func(ctx context.Context) (cloud.CredentialAPI, error)
	api               *fakeUpdateCredentialAPI
}

var _ = tc.Suite(&addCredentialSuite{})

func (s *addCredentialSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	unreg := environs.RegisterProvider("mock-addcredential-provider", &mockProvider{credSchemas: &s.schema})
	s.AddCleanup(func(_ *tc.C) {
		unreg()
	})
}

func (s *addCredentialSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.store = jujuclient.NewMemStore()
	s.store.Credentials = make(map[string]jujucloud.CloudCredential)
	s.cloudByNameFunc = func(cloud string) (*jujucloud.Cloud, error) {
		if cloud != "somecloud" && cloud != "anothercloud" {
			return nil, errors.NotFoundf("cloud %v", cloud)
		}
		return &jujucloud.Cloud{
			Name:             cloud,
			Type:             "mock-addcredential-provider",
			AuthTypes:        s.authTypes,
			Endpoint:         "cloud-endpoint",
			IdentityEndpoint: "cloud-identity-endpoint",
		}, nil
	}
	s.api = &fakeUpdateCredentialAPI{
		clouds: func() (map[names.CloudTag]jujucloud.Cloud, error) { return nil, nil },
	}
	s.credentialAPIFunc = func(ctx context.Context) (cloud.CredentialAPI, error) { return s.api, nil }
}

func (s *addCredentialSuite) runCmd(c *tc.C, stdin io.Reader, args ...string) (*cmd.Context, *cloud.AddCredentialCommand, error) {
	addCmd := cloud.NewAddCredentialCommandForTest(s.store, s.cloudByNameFunc, s.credentialAPIFunc)
	err := cmdtesting.InitCommand(addCmd, args)
	if err != nil {
		return nil, nil, err
	}
	ctx := cmdtesting.Context(c)
	ctx.Stdin = stdin
	err = addCmd.Run(ctx)
	return ctx, addCmd, err
}

func (s *addCredentialSuite) run(c *tc.C, stdin io.Reader, args ...string) (*cmd.Context, error) {
	ctx, _, err := s.runCmd(c, stdin, args...)
	return ctx, err
}

func (s *addCredentialSuite) TestBadArgs(c *tc.C) {
	_, err := s.run(c, nil)
	c.Assert(err, tc.ErrorMatches, `Usage: juju add-credential <cloud-name> \[-f <credentials.yaml>\]`)
	_, err = s.run(c, nil, "somecloud", "-f", "credential.yaml", "extra")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *addCredentialSuite) TestBadLocalCloudName(c *tc.C) {
	ctx, err := s.run(c, nil, "badcloud", "--client")
	c.Assert(err, tc.Equals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "To view all available clouds, use 'juju clouds'.\nTo add new cloud, use 'juju add-cloud'.\n")
	c.Assert(c.GetTestLog(), tc.Contains, "cloud badcloud not valid")
}

func (s *addCredentialSuite) TestAddFromFileBadFilename(c *tc.C) {
	_, err := s.run(c, nil, "somecloud", "-f", "somefile.yaml", "--client")
	c.Assert(err, tc.ErrorMatches, ".*open somefile.yaml: .*")
}

func (s *addCredentialSuite) TestNoCredentialsRequired(c *tc.C) {
	s.authTypes = nil
	_, err := s.run(c, nil, "somecloud", "--client")
	c.Assert(err, tc.ErrorMatches, `cloud "somecloud" does not require credentials`)
}

func (s *addCredentialSuite) createTestCredentialData(c *tc.C) string {
	return s.createTestCredentialDataWithAuthType(c, "access-key")
}

func (s *addCredentialSuite) createTestCredentialDataWithAuthType(c *tc.C, authType string) string {
	return s.createTestCredentialFile(c, fmt.Sprintf(`
credentials:
  somecloud:
    me:
      auth-type: %v
      access-key: <key>
      secret-key: <secret>
`[1:], authType))
}

func (s *addCredentialSuite) createTestCredentialFile(c *tc.C, content string) string {
	dir := c.MkDir()
	credsFile := filepath.Join(dir, "cred.yaml")
	err := os.WriteFile(credsFile, []byte(content), 0600)
	c.Assert(err, tc.ErrorIsNil)
	return credsFile
}

func (s *addCredentialSuite) TestAddFromFileWithInvalidCredentialNames(c *tc.C) {
	dir := c.MkDir()
	sourceFile := filepath.Join(dir, "cred.yaml")
	err := os.WriteFile(sourceFile, []byte(`
credentials:
  somecloud:
    credential with spaces:
      auth-type: interactive
      trust-password: "123"
`), 0644)
	c.Assert(err, tc.IsNil)

	s.authTypes = []jujucloud.AuthType{jujucloud.InteractiveAuthType}
	_, err = s.run(c, nil, "somecloud", "-f", sourceFile, "--client")
	c.Assert(err, tc.ErrorMatches, `"credential with spaces" is not a valid credential name`)
}

func (s *addCredentialSuite) TestAddFromFileNoCredentialsFound(c *tc.C) {
	sourceFile := s.createTestCredentialData(c)
	_, err := s.run(c, nil, "anothercloud", "-f", sourceFile, "--client")
	c.Assert(err, tc.ErrorMatches, `no credentials for cloud anothercloud exist in file.*`)
}

func (s *addCredentialSuite) TestAddFromFileExisting(c *tc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType, jujucloud.AccessKeyAuthType}
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{"cred": {}},
		},
	}
	sourceFile := s.createTestCredentialData(c)
	_, err := s.run(c, nil, "somecloud", "-f", sourceFile, "--client")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.store.Credentials, tc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"cred": {},
				"me": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{
					"access-key": "<key>",
					"secret-key": "<secret>",
				})},
		},
	})
}

func (s *addCredentialSuite) TestAddInvalidRegionSpecified(c *tc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.AccessKeyAuthType}
	_, err := s.run(c, nil, "somecloud", "--region", "someregion", "--client")
	c.Assert(err, tc.ErrorMatches, `provided region "someregion" for cloud "somecloud" not valid`)
}

func (s *addCredentialSuite) setupCloudWithRegions(c *tc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType}
	s.cloudByNameFunc = func(cloudName string) (*jujucloud.Cloud, error) {
		return &jujucloud.Cloud{
			Name:             cloudName,
			Type:             "dummy",
			AuthTypes:        s.authTypes,
			Endpoint:         "cloud-endpoint",
			IdentityEndpoint: "cloud-identity-endpoint",
			Regions: []jujucloud.Region{
				{Name: "anotherregion", Endpoint: "specialendpoint", IdentityEndpoint: "specialidentityendpoint", StorageEndpoint: "storageendpoint"},
				{Name: "specialregion", Endpoint: "specialendpoint", IdentityEndpoint: "specialidentityendpoint", StorageEndpoint: "storageendpoint"},
			},
		}, nil
	}
	c.Assert(s.store.Credentials, tc.DeepEquals, map[string]jujucloud.CloudCredential{})
}

func (s *addCredentialSuite) createFileForAddCredential(c *tc.C) string {
	dir := c.MkDir()
	credsFile := filepath.Join(dir, "cred.yaml")
	data := `
credentials:
  somecloud:
    fred:
      auth-type: userpass
      username: user
      password: password
`[1:]
	err := os.WriteFile(credsFile, []byte(data), 0600)
	c.Assert(err, tc.ErrorIsNil)
	return credsFile
}

func (s *addCredentialSuite) TestAddWithFileRegionSpecified(c *tc.C) {
	s.setupCloudWithRegions(c)
	args := []string{"somecloud", "-f", s.createFileForAddCredential(c), "--client"}
	s.assertCredentialAdded(c, "", args, "specialregion", "specialregion")
}

func (s *addCredentialSuite) TestAddWithFileNoRegionSpecified(c *tc.C) {
	s.setupCloudWithRegions(c)
	args := []string{"somecloud", "-f", s.createFileForAddCredential(c), "--client"}
	s.assertCredentialAdded(c, "", args, "", "")
}

func (s *addCredentialSuite) TestAddInteractiveNoRegionSpecified(c *tc.C) {
	s.setupCloudWithRegions(c)
	args := []string{"somecloud", "--client"}

	ctxt := s.assertCredentialAdded(c, "fred\n\nuser\npassword\n", args, "", "")
	c.Assert(cmdtesting.Stdout(ctxt), tc.Equals, `
Enter credential name: 
Regions
  anotherregion
  specialregion

Select region [any region, credential is not region specific]: 
Using auth-type "userpass".

Enter username: 
Enter password: 
Credential "fred" added locally for cloud "somecloud".

`[1:])
}

func (s *addCredentialSuite) TestAddInteractiveInvalidRegionEntered(c *tc.C) {
	s.setupCloudWithRegions(c)
	args := []string{"somecloud", "--client"}

	ctxt := s.assertCredentialAdded(c, "fred\nnotknownregion\n\nuser\npassword\n", args, "", "")
	c.Assert(cmdtesting.Stdout(ctxt), tc.Equals, `
Enter credential name: 
Regions
  anotherregion
  specialregion

Select region [any region, credential is not region specific]: provided region "notknownregion" for cloud "somecloud" not valid

Select region [any region, credential is not region specific]: 
Using auth-type "userpass".

Enter username: 
Enter password: 
Credential "fred" added locally for cloud "somecloud".

`[1:])
}

func (s *addCredentialSuite) TestAddInteractiveRegionSpecified(c *tc.C) {
	s.setupCloudWithRegions(c)
	args := []string{"somecloud", "--client"}

	ctxt := s.assertCredentialAdded(c, "fred\nuser\npassword\n", args, "specialregion", "specialregion")
	c.Assert(cmdtesting.Stdout(ctxt), tc.Equals, `
Enter credential name: 
User specified region "specialregion", using it.

Using auth-type "userpass".

Enter username: 
Enter password: 
Credential "fred" added locally for cloud "somecloud".

`[1:])
}

func (s *addCredentialSuite) assertCredentialAdded(c *tc.C, input string, args []string, specifiedRegion, expectedRegion string) *cmd.Context {
	var stdin *strings.Reader
	if input != "" {
		stdin = strings.NewReader(input)
	}
	if specifiedRegion != "" {
		args = append(args, "--region", specifiedRegion)
	}

	ctxt, runCmd, err := s.runCmd(c, stdin, args...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(runCmd.Region, tc.Equals, expectedRegion)
	c.Assert(s.store.Credentials, tc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"fred": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{
					"username": "user",
					"password": "password",
				})},
		},
	})
	return ctxt
}

func (s *addCredentialSuite) TestAddNewFromFile(c *tc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.AccessKeyAuthType}
	sourceFile := s.createTestCredentialData(c)
	_, err := s.run(c, nil, "somecloud", "-f", sourceFile, "--client")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.store.Credentials, tc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"me": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{
					"access-key": "<key>",
					"secret-key": "<secret>",
				})},
		},
	})
}

func (s *addCredentialSuite) TestAddInvalidAuth(c *tc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.AccessKeyAuthType}
	sourceFile := s.createTestCredentialDataWithAuthType(c, "invalid auth")
	_, err := s.run(c, nil, "somecloud", "-f", sourceFile, "--client")
	c.Assert(err, tc.ErrorMatches,
		regexp.QuoteMeta(`credential "me" contains invalid auth type "invalid auth", valid auth types for cloud "somecloud" are [access-key]`))
}

func (s *addCredentialSuite) TestAddCloudUnsupportedAuth(c *tc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.AccessKeyAuthType}
	sourceFile := s.createTestCredentialDataWithAuthType(c, fmt.Sprintf("%v", jujucloud.JSONFileAuthType))
	_, err := s.run(c, nil, "somecloud", "-f", sourceFile, "--client")
	c.Assert(err, tc.ErrorMatches,
		regexp.QuoteMeta(`credential "me" contains invalid auth type "jsonfile", valid auth types for cloud "somecloud" are [access-key]`))
}

func (s *addCredentialSuite) assertAddUserpassCredential(c *tc.C, input string, expected *jujucloud.Credential, msg string) {
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
	ctx, err := s.run(c, stdin, "somecloud", "--client")
	c.Assert(err, tc.ErrorIsNil)
	var cred jujucloud.Credential
	if expected == nil {
		cred = jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{
			"username": "user",
			"password": "password",
		})
	} else {
		cred = *expected
	}
	c.Assert(s.store.Credentials, tc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"fred": cred,
			},
		},
	})
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, msg)
}

func (s *addCredentialSuite) TestAddCredentialSingleAuthType(c *tc.C) {
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

func (s *addCredentialSuite) TestAddCredentialRetryOnMissingMandatoryAttribute(c *tc.C) {
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

func (s *addCredentialSuite) TestAddCredentialMultipleAuthType(c *tc.C) {
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

func (s *addCredentialSuite) TestAddCredentialInteractive(c *tc.C) {
	s.authTypes = []jujucloud.AuthType{"interactive"}
	s.schema = map[jujucloud.AuthType]jujucloud.CredentialSchema{
		"interactive": {{"username", jujucloud.CredentialAttr{}}},
	}

	stdin := strings.NewReader("bobscreds\nbob\n")
	ctx, err := s.run(c, stdin, "somecloud", "--client")
	c.Assert(err, tc.ErrorIsNil)

	// there's an extra line return after Using auth-type because the rest get a
	// second line return from the user hitting return when they enter a value
	// (which is not shown here), but that one does not.
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
Enter credential name: 
Using auth-type "interactive".

Enter username: 
Credential "bobscreds" added locally for cloud "somecloud".

`[1:])

	// FinalizeCredential should have generated a userpass credential
	// based on the input from the interactive credential.
	c.Assert(s.store.Credentials, tc.DeepEquals, map[string]jujucloud.CloudCredential{
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

func (s *addCredentialSuite) TestAddCredentialInteractiveHiddenFile(c *tc.C) {
	s.authTypes = []jujucloud.AuthType{"interactive"}
	s.schema = map[jujucloud.AuthType]jujucloud.CredentialSchema{
		"interactive": {
			{
				Name: "username",
				CredentialAttr: jujucloud.CredentialAttr{
					Description:    "the path to the username file",
					ExpandFilePath: true,
					Hidden:         true,
				},
			},
		},
	}

	file, err := os.CreateTemp("", "username-file")
	c.Assert(err, tc.ErrorIsNil)
	defer file.Close()
	_, err = file.WriteString("test")
	c.Assert(err, tc.ErrorIsNil)

	stdin := strings.NewReader(fmt.Sprintf("wallyworld\n%s\n", file.Name()))
	ctx, err := s.run(c, stdin, "somecloud", "--client")
	c.Assert(err, tc.ErrorIsNil)

	// there's an extra line return after Using auth-type because the rest get a
	// second line return from the user hitting return when they enter a value
	// (which is not shown here), but that one does not.
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
Enter credential name: 
Using auth-type "interactive".

Enter the path to the username file: 
Credential "wallyworld" added locally for cloud "somecloud".

`[1:])

	c.Assert(s.store.Credentials, tc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"wallyworld": jujucloud.NewCredential("userpass", map[string]string{
					"application-password": "cloud-identity-endpoint",
					"password":             "cloud-endpoint",
					"username":             "test",
				}),
			},
		},
	})
}

func (s *addCredentialSuite) TestAddInvalidCredentialInteractive(c *tc.C) {
	s.authTypes = []jujucloud.AuthType{"interactive"}
	s.schema = map[jujucloud.AuthType]jujucloud.CredentialSchema{
		"interactive": {{"username", jujucloud.CredentialAttr{}}},
	}

	stdin := strings.NewReader("credential name with spaces\n")
	ctx, err := s.run(c, stdin, "somecloud", "--client")
	c.Assert(err, tc.NotNil)

	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
Enter credential name: Invalid credential name: "credential name with spaces"

Enter credential name: 
`[1:])
}

func (s *addCredentialSuite) TestAddCredentialCredSchemaInteractive(c *tc.C) {
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
	ctx, err := s.run(c, stdin, "somecloud", "--client")
	c.Assert(err, tc.ErrorIsNil)

	// there's an extra line return after Using auth-type because the rest get a
	// second line return from the user hitting return when they enter a value
	// (which is not shown here), but that one does not.
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
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
	c.Assert(s.store.Credentials, tc.DeepEquals, map[string]jujucloud.CloudCredential{
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

func (s *addCredentialSuite) TestAddCredentialReplace(c *tc.C) {
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

func (s *addCredentialSuite) TestAddCredentialReplaceDecline(c *tc.C) {
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

func (s *addCredentialSuite) assertAddFileCredential(c *tc.C, input, fileKey string) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "jsonfile")
	err := os.WriteFile(filename, []byte{}, 0600)
	c.Assert(err, tc.ErrorIsNil)

	stdin := strings.NewReader(fmt.Sprintf(input, filename))
	addCmd := cloud.NewAddCredentialCommandForTest(s.store, s.cloudByNameFunc, s.credentialAPIFunc)
	err = cmdtesting.InitCommand(addCmd, []string{"somecloud", "--client"})
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.ContextForDir(c, dir)
	ctx.Stdin = stdin
	err = addCmd.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.store.Credentials, tc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"fred": jujucloud.NewCredential(s.authTypes[0], map[string]string{
					fileKey: filename,
				}),
			},
		},
	})
}

func (s *addCredentialSuite) TestAddJsonFileCredential(c *tc.C) {
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

func (s *addCredentialSuite) TestAddCredentialWithFileAttr(c *tc.C) {
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

func (s *addCredentialSuite) assertAddCredentialWithOptions(c *tc.C, input string) {
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
	_, err := s.run(c, stdin, "somecloud", "--client")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.store.Credentials, tc.DeepEquals, map[string]jujucloud.CloudCredential{
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

func (s *addCredentialSuite) TestAddCredentialWithOptions(c *tc.C) {
	s.assertAddCredentialWithOptions(c, "fred\nuser\nbadoption\noptionA\n")
}

func (s *addCredentialSuite) TestAddCredentialWithOptionsAutofill(c *tc.C) {
	s.assertAddCredentialWithOptions(c, "fred\nuser\n\n")
}

func (s *addCredentialSuite) TestAddMAASCredential(c *tc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.OAuth1AuthType}
	s.schema = map[jujucloud.AuthType]jujucloud.CredentialSchema{
		jujucloud.OAuth1AuthType: {
			{
				"maas-oauth", jujucloud.CredentialAttr{},
			},
		},
	}
	stdin := strings.NewReader("fred\nauth:token\n")
	_, err := s.run(c, stdin, "somecloud", "--client")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.store.Credentials, tc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"fred": jujucloud.NewCredential(jujucloud.OAuth1AuthType, map[string]string{
					"maas-oauth": "auth:token",
				}),
			},
		},
	})
}

func (s *addCredentialSuite) TestAddGCEFileCredentials(c *tc.C) {
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
	ctx, err := s.run(c, stdin, "somecloud", "--client")
	c.Assert(err, tc.ErrorIsNil)
	expected := `
Enter credential name: 
Using auth-type "jsonfile".

Enter path to the credential file: 
Credential "blah" added locally for cloud "somecloud".

`[1:]
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, expected)
}

func (s *addCredentialSuite) TestShouldFinalizeCredentialWithEnvironProvider(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	provider := environsTesting.NewMockEnvironProvider(ctrl)
	cred := jujucloud.Credential{}
	got := cloud.ShouldFinalizeCredential(provider, cred)
	c.Assert(got, tc.IsFalse)
}

func (s *addCredentialSuite) TestShouldFinalizeCredentialSuccess(c *tc.C) {
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
	c.Assert(got, tc.IsTrue)
}

func (s *addCredentialSuite) TestShouldFinalizeCredentialFailure(c *tc.C) {
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
	c.Assert(got, tc.IsFalse)
}

func (s *addCredentialSuite) setupStore(c *tc.C) {
	s.store.Controllers["controller"] = jujuclient.ControllerDetails{ControllerUUID: "cdcssc"}
	s.store.CurrentControllerName = "controller"
	s.store.Accounts = map[string]jujuclient.AccountDetails{
		"controller": {
			User: "admin@local",
		},
	}
}

func (s *addCredentialSuite) TestAddRemoteFromFile(c *tc.C) {
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
	s.api.clouds = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag("somecloud"): {
				Name:             "somecloud",
				Type:             "mock-addcredential-provider",
				AuthTypes:        s.authTypes,
				Endpoint:         "cloud-endpoint",
				IdentityEndpoint: "cloud-identity-endpoint",
			},
		}, nil
	}
	stdout := `
Enter credential name: 
Using auth-type "jsonfile".

Enter path to the credential file: 
`[1:]
	stderr := `
Using cloud "somecloud" from the controller to verify credentials.
Controller credential "blah" for user "admin@local" for cloud "somecloud" on controller "controller" added.
For more information, see 'juju show-credential somecloud blah'.
`[1:]

	s.assertAddedCredentialForCloudWithArgs(c, "somecloud", stdout, "", stderr, true, false, "--c", "controller")
}

func (s *addCredentialSuite) assertAddedCredentialForCloudWithArgs(c *tc.C, cloudName, expectedStdout, expectedStdin, expectedStderr string, uploaded, added bool, args ...string) {
	s.setupStore(c)
	expectedContents := fmt.Sprintf(`
credentials:
  %v:
    blah:
      auth-type: jsonfile
      access-key: <key>
      secret-key: <secret>
`[1:], cloudName)
	sourceFile := s.createTestCredentialFile(c, expectedContents)

	called := false
	s.api.addCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential) ([]params.UpdateCredentialResult, error) {
		c.Assert(cloudCredentials, tc.HasLen, 1)
		called = true
		expectedTag := names.NewCloudCredentialTag(fmt.Sprintf("%v/admin@local/blah", cloudName)).String()
		for k, v := range cloudCredentials {
			c.Assert(k, tc.DeepEquals, expectedTag)
			c.Assert(v.Attributes()["file"], tc.Equals, expectedContents)
		}
		return []params.UpdateCredentialResult{{CredentialTag: expectedTag}}, nil
	}

	stdin := strings.NewReader(fmt.Sprintf("%vblah\n%s\n", expectedStdin, sourceFile))

	ctx, err := s.run(c, stdin, append(args, cloudName)...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, expectedStdout)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, expectedStderr)

	if added {
		c.Assert(s.store.Credentials[cloudName].AuthCredentials["blah"].Attributes()["file"], tc.Not(tc.Contains), expectedContents)
		c.Assert(s.store.Credentials[cloudName].AuthCredentials["blah"].Attributes()["file"], tc.Equals, sourceFile)
	}
	c.Assert(called, tc.Equals, uploaded)
}

func (s *addCredentialSuite) TestAddRemoteCloudOnlyNoLocal(c *tc.C) {
	s.api.clouds = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag("remote"): {
				Name:      "remote",
				Type:      "gce",
				AuthTypes: []jujucloud.AuthType{jujucloud.JSONFileAuthType},
			},
		}, nil
	}
	stdout := `
Enter credential name: 
Using auth-type "jsonfile".

Enter path to the .json file containing a service account key for your project
Path: 
`[1:]
	stderr := `
Using cloud "remote" from the controller to verify credentials.
Controller credential "blah" for user "admin@local" for cloud "remote" on controller "controller" added.
For more information, see 'juju show-credential remote blah'.
`[1:]
	s.assertAddedCredentialForCloudWithArgs(c, "remote", stdout, "", stderr, true, false, "--c", "controller")
}

func (s *addCredentialSuite) TestAddRemoteNoRemoteCloud(c *tc.C) {
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
	stdout := `
Enter credential name: 
Using auth-type "jsonfile".

Enter path to the credential file: 
No cloud "somecloud" found on the controller "controller": credentials are not uploaded.
Use 'juju clouds -c controller' to see what clouds are available on the controller.
User 'juju add-cloud somecloud -c controller' to add your cloud to the controller.
`[1:]
	s.assertAddedCredentialForCloudWithArgs(c, "somecloud", stdout, "", "", false, false, "--c", "controller")
}

func (s *addCredentialSuite) TestAddRemoteCloudPromptForController(c *tc.C) {
	s.api.clouds = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag("remote"): {
				Name:      "remote",
				Type:      "gce",
				AuthTypes: []jujucloud.AuthType{jujucloud.JSONFileAuthType},
			},
		}, nil
	}
	stdout := `
Do you want to add a credential to:
    1. client only (--client)
    2. controller "controller" only (--controller controller)
    3. both (--client --controller controller)
Enter your choice, or type Q|q to quit: Enter credential name: 
Using auth-type "jsonfile".

Enter path to the .json file containing a service account key for your project
Path: 
Credential "blah" added locally for cloud "remote".

`[1:]
	stderr := `
This operation can be applied to both a copy on this client and to the one on a controller.
Using cloud "remote" from the controller to verify credentials.
Controller credential "blah" for user "admin@local" for cloud "remote" on controller "controller" added.
For more information, see 'juju show-credential remote blah'.
`[1:]
	s.assertAddedCredentialForCloudWithArgs(c, "remote", stdout, "3\n", stderr, true, true)
}

func (s *addCredentialSuite) TestAddRemoteCloudControllerOnly(c *tc.C) {
	s.api.clouds = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag("remote"): {
				Name:      "remote",
				Type:      "gce",
				AuthTypes: []jujucloud.AuthType{jujucloud.JSONFileAuthType},
			},
		}, nil
	}
	stdout := `
Enter credential name: 
Using auth-type "jsonfile".

Enter path to the .json file containing a service account key for your project
Path: 
`[1:]
	stderr := `
Using cloud "remote" from the controller to verify credentials.
Controller credential "blah" for user "admin@local" for cloud "remote" on controller "controller" added.
For more information, see 'juju show-credential remote blah'.
`[1:]
	s.assertAddedCredentialForCloudWithArgs(c, "remote", stdout, "", stderr, true, false, "-c", "controller")
}
