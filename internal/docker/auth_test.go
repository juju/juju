// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker_test

import (
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/testhelpers"
)

type authSuite struct {
	testhelpers.IsolationSuite
}

func TestAuthSuite(t *stdtesting.T) {
	tc.Run(t, &authSuite{})
}

var (
	ecrContent = `
{
    "serveraddress": "66668888.dkr.ecr.eu-west-1.amazonaws.com",
    "username": "aws_access_key_id",
    "repository": "test-account",
    "password": "aws_secret_access_key",
    "identitytoken": "xxxxx==",
    "region": "ap-southeast-2"
}`[1:]

	quayContent = `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}
`[1:]
)

func (s *authSuite) TestNewImageRepoDetailsReadFromFile(c *tc.C) {
	filename := "my-caas-image-repo-config.json"
	dir := c.MkDir()
	fullpath := filepath.Join(dir, filename)
	err := os.WriteFile(fullpath, []byte(quayContent), 0644)
	c.Assert(err, tc.ErrorIsNil)
	imageRepoDetails, err := docker.LoadImageRepoDetails(fullpath)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(imageRepoDetails, tc.DeepEquals, docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	})
}

func (s *authSuite) TestNewImageRepoDetailsReadFromContent(c *tc.C) {
	imageRepoDetails, err := docker.NewImageRepoDetails(quayContent)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(imageRepoDetails, tc.DeepEquals, docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	})

	imageRepoDetails, err = docker.NewImageRepoDetails(ecrContent)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(imageRepoDetails, tc.DeepEquals, docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Region:        "ap-southeast-2",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "aws_access_key_id",
			Password: "aws_secret_access_key",
		},
		TokenAuthConfig: docker.TokenAuthConfig{
			IdentityToken: docker.NewToken("xxxxx=="),
		},
	})
}

func (s *authSuite) TestNewImageRepoDetailsReadDefaultServerAddress(c *tc.C) {
	data := `
{
    "auth": "xxxxx==",
    "repository": "qabot"
}
`[1:]
	imageRepoDetails, err := docker.NewImageRepoDetails(data)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(imageRepoDetails, tc.DeepEquals, docker.ImageRepoDetails{
		Repository: "qabot",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	})
}

func (s *authSuite) TestValidateImageRepoDetails(c *tc.C) {
	imageRepoDetails := docker.ImageRepoDetails{}
	c.Assert(imageRepoDetails.Validate(), tc.ErrorMatches, `empty repository not valid`)

	imageRepoDetails = docker.ImageRepoDetails{
		Repository: "bad repo",
	}
	c.Assert(imageRepoDetails.Validate(), tc.ErrorMatches, `docker image path "bad repo": invalid reference format`)
}

func (s *authSuite) TestSecretData(c *tc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository:    "quay.io/test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	}
	data, err := imageRepoDetails.SecretData()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.DeepEquals, `{"auths":{"quay.io":{"auth":"xxxxx==","username":"","password":"","serveraddress":"quay.io"}}}`)

	imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
	}
	data, err = imageRepoDetails.SecretData()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(data), tc.DeepEquals, 0)
}

func (s *authSuite) TestIsPrivate(c *tc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	}
	c.Assert(imageRepoDetails.IsPrivate(), tc.DeepEquals, true)

	imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
	}
	c.Assert(imageRepoDetails.IsPrivate(), tc.DeepEquals, false)
}

func (s *authSuite) TestAuthEqual(c *tc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	}
	c.Assert(imageRepoDetails.AuthEqual(imageRepoDetails), tc.DeepEquals, true)

	imageRepoDetails2 := docker.ImageRepoDetails{
		Repository: "test-account",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	}
	c.Assert(imageRepoDetails.AuthEqual(imageRepoDetails2), tc.DeepEquals, true)

	imageRepoDetails3 := docker.ImageRepoDetails{
		Repository:      "test-account",
		ServerAddress:   "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{},
	}
	c.Assert(imageRepoDetails.AuthEqual(imageRepoDetails3), tc.DeepEquals, false)
}

func (s *authSuite) TestTokenAuthConfigEmpty(c *tc.C) {
	cfg := docker.TokenAuthConfig{}
	c.Assert(cfg.Empty(), tc.DeepEquals, true)

	cfg = docker.TokenAuthConfig{
		IdentityToken: docker.NewToken("xxx"),
	}
	c.Assert(cfg.Empty(), tc.DeepEquals, false)
}

func (s *authSuite) TestBasicAuthConfigEmpty(c *tc.C) {
	cfg := docker.BasicAuthConfig{}
	c.Assert(cfg.Empty(), tc.DeepEquals, true)

	cfg = docker.BasicAuthConfig{
		Auth: docker.NewToken("xxxx=="),
	}
	c.Assert(cfg.Empty(), tc.DeepEquals, false)
	cfg = docker.BasicAuthConfig{
		Username: "xxx",
	}
	c.Assert(cfg.Empty(), tc.DeepEquals, false)
	cfg = docker.BasicAuthConfig{
		Password: "xxx",
	}
	c.Assert(cfg.Empty(), tc.DeepEquals, false)
}

func (s *authSuite) TestToken(c *tc.C) {
	token := docker.NewToken("xxxx==")
	c.Assert(token, tc.DeepEquals, &docker.Token{Value: "xxxx=="})
	c.Assert(token.String(), tc.DeepEquals, `******`)
	c.Assert(token.Content(), tc.DeepEquals, `xxxx==`)
	c.Assert(token.Empty(), tc.IsFalse)
	data, err := token.MarshalJSON()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, []byte(`"xxxx=="`))

	token.Value = ""
	c.Assert(token.Empty(), tc.IsTrue)

	token = docker.NewToken("")
	c.Assert(token.Empty(), tc.IsTrue)
	c.Assert(token, tc.IsNil)
}
