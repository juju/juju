// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"fmt"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type addCredentialSuite struct {
	testing.BaseSuite

	store           *jujuclienttesting.MemStore
	schema          map[jujucloud.AuthType]jujucloud.CredentialSchema
	authTypes       []jujucloud.AuthType
	cloudByNameFunc func(string) (*jujucloud.Cloud, error)
}

var _ = gc.Suite(&addCredentialSuite{
	store: jujuclienttesting.NewMemStore(),
})

func (s *addCredentialSuite) SetUpSuite(c *gc.C) {
	environs.RegisterProvider("mock-addcredential-provider", &mockProvider{credSchemas: &s.schema})
	s.cloudByNameFunc = func(cloud string) (*jujucloud.Cloud, error) {
		if cloud != "somecloud" && cloud != "anothercloud" {
			return nil, errors.NotFoundf("cloud %v", cloud)
		}
		return &jujucloud.Cloud{
			Type:      "mock-addcredential-provider",
			AuthTypes: s.authTypes,
		}, nil
	}
}

func (s *addCredentialSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.store.Credentials = make(map[string]jujucloud.CloudCredential)
}

func (s *addCredentialSuite) run(c *gc.C, stdin io.Reader, args ...string) (*cmd.Context, error) {
	addCmd := cloud.NewAddCredentialCommandForTest(s.store, s.cloudByNameFunc)
	err := testing.InitCommand(addCmd, args)
	if err != nil {
		return nil, err
	}
	ctx := testing.Context(c)
	ctx.Stdin = stdin
	return ctx, addCmd.Run(ctx)
}

func (s *addCredentialSuite) TestBadArgs(c *gc.C) {
	_, err := s.run(c, nil)
	c.Assert(err, gc.ErrorMatches, `Usage: juju add-credential <cloud-name> \[-f <credentials.yaml>\]`)
	_, err = s.run(c, nil, "somecloud", "-f", "credential.yaml", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
	_, err = s.run(c, nil, "bad-cloud", "-f", "credential.yaml")
	c.Assert(err, gc.ErrorMatches, `cloud bad-cloud not valid`)
}

func (s *addCredentialSuite) TestAddFromFileBadFilename(c *gc.C) {
	_, err := s.run(c, nil, "somecloud", "-f", "somefile.yaml")
	c.Assert(err, gc.ErrorMatches, ".*open somefile.yaml: .*")
}

func (s *addCredentialSuite) createTestCredentialData(c *gc.C) string {
	dir := c.MkDir()
	credsFile := filepath.Join(dir, "cred.yaml")
	data := `
credentials:
  somecloud:
    me:
      auth-type: access-key
      access-key: <key>
      secret-key: <secret>
`[1:]
	err := ioutil.WriteFile(credsFile, []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)
	return credsFile
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
	c.Assert(err, gc.ErrorMatches, `credentials for cloud somecloud already exist; use --replace to overwrite / merge`)
}

func (s *addCredentialSuite) TestAddFromFileExistingReplace(c *gc.C) {
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

// TODO(wallyworld) - these tests should also validate that the prompts and messages are as expected.

func (s *addCredentialSuite) assertAddUserpassCredential(c *gc.C, input string) {
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
	_, err := s.run(c, stdin, "somecloud")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.store.Credentials, jc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"fred": jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{
					"username": "user",
					"password": "password",
				}),
			},
		},
	})
}

func (s *addCredentialSuite) TestAddCredentialSingleAuthType(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType}
	s.assertAddUserpassCredential(c, "fred\nuser\npassword\n")
}

func (s *addCredentialSuite) TestAddCredentialRetryOnMissingMandatoryAttribute(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType}
	s.assertAddUserpassCredential(c, "fred\n\nuser\npassword\n")
}

func (s *addCredentialSuite) TestAddCredentialMultipleAuthType(c *gc.C) {
	s.authTypes = []jujucloud.AuthType{jujucloud.UserPassAuthType, jujucloud.AccessKeyAuthType}
	s.assertAddUserpassCredential(c, "fred\nuserpass\nuser\npassword\n")
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
	dir := c.MkDir()
	filename := filepath.Join(dir, "jsonfile")
	err := ioutil.WriteFile(filename, []byte{}, 0600)
	c.Assert(err, jc.ErrorIsNil)

	// Input includes invalid file info.
	stdin := strings.NewReader(fmt.Sprintf("fred\nbadfile\n.\n%s\n", filename))
	addCmd := cloud.NewAddCredentialCommandForTest(s.store, s.cloudByNameFunc)
	err = testing.InitCommand(addCmd, []string{"somecloud"})
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.ContextForDir(c, dir)
	ctx.Stdin = stdin
	err = addCmd.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.store.Credentials, jc.DeepEquals, map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"fred": jujucloud.NewCredential(jujucloud.JSONFileAuthType, map[string]string{
					"file": filename,
				}),
			},
		},
	})
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
