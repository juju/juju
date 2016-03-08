// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type addCredentialSuite struct {
	testing.BaseSuite

	store           *jujuclienttesting.MemStore
	cloudByNameFunc func(string) (*jujucloud.Cloud, error)
}

var _ = gc.Suite(&addCredentialSuite{
	store: jujuclienttesting.NewMemStore(),
	cloudByNameFunc: func(cloud string) (*jujucloud.Cloud, error) {
		if cloud != "somecloud" && cloud != "anothercloud" {
			return nil, errors.NotFoundf("cloud %v", cloud)
		}
		return &jujucloud.Cloud{Type: "dummy"}, nil
	},
})

func (s *addCredentialSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.store.Credentials = make(map[string]jujucloud.CloudCredential)
}

func (s *addCredentialSuite) run(c *gc.C, args ...string) error {
	addCmd := cloud.NewAddCredentialCommandForTest(s.store, s.cloudByNameFunc)
	_, err := testing.RunCommand(c, addCmd, args...)
	return err
}

func (s *addCredentialSuite) TestBadArgs(c *gc.C) {
	err := s.run(c)
	c.Assert(err, gc.ErrorMatches, "Usage: juju add-credential <cloud-name> -f <credentials.yaml>")
	err = s.run(c, "somecloud")
	c.Assert(err, gc.ErrorMatches, `Usage: juju add-credential <cloud-name> -f <credentials.yaml>`)
	err = s.run(c, "somecloud", "-f", "credential.yaml", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
	err = s.run(c, "bad-cloud", "-f", "credential.yaml")
	c.Assert(err, gc.ErrorMatches, `cloud bad-cloud not valid`)
}

func (s *addCredentialSuite) TestAddBadFilename(c *gc.C) {
	err := s.run(c, "somecloud", "-f", "somefile.yaml")
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

func (s *addCredentialSuite) TestAddNoCredentialsFound(c *gc.C) {
	sourceFile := s.createTestCredentialData(c)
	err := s.run(c, "anothercloud", "-f", sourceFile)
	c.Assert(err, gc.ErrorMatches, `no credentials for cloud anothercloud exist in file.*`)
}

func (s *addCredentialSuite) TestAddExisting(c *gc.C) {
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{"cred": {}},
		},
	}
	sourceFile := s.createTestCredentialData(c)
	err := s.run(c, "somecloud", "-f", sourceFile)
	c.Assert(err, gc.ErrorMatches, `credentials for cloud somecloud already exist; use --replace to overwrite / merge`)
}

func (s *addCredentialSuite) TestAddExistingReplace(c *gc.C) {
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"somecloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"cred": jujucloud.NewCredential(jujucloud.UserPassAuthType, nil)},
		},
	}
	sourceFile := s.createTestCredentialData(c)
	err := s.run(c, "somecloud", "-f", sourceFile, "--replace")
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

func (s *addCredentialSuite) TestAddNew(c *gc.C) {
	sourceFile := s.createTestCredentialData(c)
	err := s.run(c, "somecloud", "-f", sourceFile)
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
