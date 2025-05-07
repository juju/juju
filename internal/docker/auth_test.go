// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker_test

import (
	"os"
	"path/filepath"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/docker"
)

type authSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&authSuite{})

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
	c.Assert(err, jc.ErrorIsNil)
	imageRepoDetails, err := docker.LoadImageRepoDetails(fullpath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageRepoDetails, jc.DeepEquals, docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	})
}

func (s *authSuite) TestNewImageRepoDetailsReadFromContent(c *tc.C) {
	imageRepoDetails, err := docker.NewImageRepoDetails(quayContent)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageRepoDetails, jc.DeepEquals, docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	})

	imageRepoDetails, err = docker.NewImageRepoDetails(ecrContent)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageRepoDetails, jc.DeepEquals, docker.ImageRepoDetails{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageRepoDetails, jc.DeepEquals, docker.ImageRepoDetails{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), jc.DeepEquals, `{"auths":{"quay.io":{"auth":"xxxxx==","username":"","password":"","serveraddress":"quay.io"}}}`)

	imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
	}
	data, err = imageRepoDetails.SecretData()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(data), jc.DeepEquals, 0)
}

func (s *authSuite) TestIsPrivate(c *tc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	}
	c.Assert(imageRepoDetails.IsPrivate(), jc.DeepEquals, true)

	imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
	}
	c.Assert(imageRepoDetails.IsPrivate(), jc.DeepEquals, false)
}

func (s *authSuite) TestAuthEqual(c *tc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	}
	c.Assert(imageRepoDetails.AuthEqual(imageRepoDetails), jc.DeepEquals, true)

	imageRepoDetails2 := docker.ImageRepoDetails{
		Repository: "test-account",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	}
	c.Assert(imageRepoDetails.AuthEqual(imageRepoDetails2), jc.DeepEquals, true)

	imageRepoDetails3 := docker.ImageRepoDetails{
		Repository:      "test-account",
		ServerAddress:   "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{},
	}
	c.Assert(imageRepoDetails.AuthEqual(imageRepoDetails3), jc.DeepEquals, false)
}

func (s *authSuite) TestTokenAuthConfigEmpty(c *tc.C) {
	cfg := docker.TokenAuthConfig{}
	c.Assert(cfg.Empty(), jc.DeepEquals, true)

	cfg = docker.TokenAuthConfig{
		IdentityToken: docker.NewToken("xxx"),
	}
	c.Assert(cfg.Empty(), jc.DeepEquals, false)
}

func (s *authSuite) TestBasicAuthConfigEmpty(c *tc.C) {
	cfg := docker.BasicAuthConfig{}
	c.Assert(cfg.Empty(), jc.DeepEquals, true)

	cfg = docker.BasicAuthConfig{
		Auth: docker.NewToken("xxxx=="),
	}
	c.Assert(cfg.Empty(), jc.DeepEquals, false)
	cfg = docker.BasicAuthConfig{
		Username: "xxx",
	}
	c.Assert(cfg.Empty(), jc.DeepEquals, false)
	cfg = docker.BasicAuthConfig{
		Password: "xxx",
	}
	c.Assert(cfg.Empty(), jc.DeepEquals, false)
}

func (s *authSuite) TestToken(c *tc.C) {
	token := docker.NewToken("xxxx==")
	c.Assert(token, tc.DeepEquals, &docker.Token{Value: "xxxx=="})
	c.Assert(token.String(), jc.DeepEquals, `******`)
	c.Assert(token.Content(), jc.DeepEquals, `xxxx==`)
	c.Assert(token.Empty(), jc.IsFalse)
	data, err := token.MarshalJSON()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, []byte(`"xxxx=="`))

	token.Value = ""
	c.Assert(token.Empty(), jc.IsTrue)

	token = docker.NewToken("")
	c.Assert(token.Empty(), jc.IsTrue)
	c.Assert(token, tc.IsNil)
}
