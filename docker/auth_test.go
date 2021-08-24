// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/feature"
	coretesting "github.com/juju/juju/testing"
)

type authSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite
}

var _ = gc.Suite(&authSuite{})

func (s *authSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.PrivateRegistry)
}

func (s *authSuite) TearDownTest(c *gc.C) {
	s.IsolationSuite.TearDownTest(c)
	s.JujuOSEnvSuite.TearDownTest(c)
}

var quay_io = `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}
`[1:]

func (s *authSuite) TestNewImageRepoDetailsReadFromFile(c *gc.C) {
	filename := "my-caas-image-repo-config.json"
	dir := c.MkDir()
	fullpath := filepath.Join(dir, filename)
	err := ioutil.WriteFile(fullpath, []byte(quay_io), 0644)
	c.Assert(err, jc.ErrorIsNil)
	imageRepoDetails, err := docker.NewImageRepoDetails(fullpath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageRepoDetails, jc.DeepEquals, &docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: "xxxxx==",
		},
	})
}

func (s *authSuite) TestNewImageRepoDetailsReadFromContent(c *gc.C) {
	imageRepoDetails, err := docker.NewImageRepoDetails(quay_io)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageRepoDetails, jc.DeepEquals, &docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: "xxxxx==",
		},
	})
}

func (s *authSuite) TestNewImageRepoDetailsReadDefaultServerAddress(c *gc.C) {
	data := `
{
    "auth": "xxxxx==",
    "repository": "qabot"
}
`[1:]
	imageRepoDetails, err := docker.NewImageRepoDetails(data)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageRepoDetails, jc.DeepEquals, &docker.ImageRepoDetails{
		Repository:    "qabot",
		ServerAddress: "https://index.docker.io/v1/",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: "xxxxx==",
		},
	})
}

func (s *authSuite) TestValidateImageRepoDetails(c *gc.C) {
	imageRepoDetails := docker.ImageRepoDetails{}
	c.Assert(imageRepoDetails.Validate(), gc.ErrorMatches, `empty repository not valid`)

	imageRepoDetails = docker.ImageRepoDetails{
		Repository: "bad repo",
	}
	c.Assert(imageRepoDetails.Validate(), gc.ErrorMatches, `docker image path "bad repo": invalid reference format`)
}

func (s *authSuite) TestSecretData(c *gc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: "xxxxx==",
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

func (s *authSuite) TestIsPrivate(c *gc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: "xxxxx==",
		},
	}
	c.Assert(imageRepoDetails.IsPrivate(), jc.DeepEquals, true)

	imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
	}
	c.Assert(imageRepoDetails.IsPrivate(), jc.DeepEquals, false)
}

func (s *authSuite) TestAuthEqual(c *gc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: "xxxxx==",
		},
	}
	c.Assert(imageRepoDetails.AuthEqual(imageRepoDetails), jc.DeepEquals, true)

	imageRepoDetails2 := docker.ImageRepoDetails{
		Repository: "test-account",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: "xxxxx==",
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

func (s *authSuite) TestTokenAuthConfigEmpty(c *gc.C) {
	cfg := docker.TokenAuthConfig{}
	c.Assert(cfg.Empty(), jc.DeepEquals, true)

	cfg = docker.TokenAuthConfig{
		IdentityToken: "xxx",
	}
	c.Assert(cfg.Empty(), jc.DeepEquals, false)
}

func (s *authSuite) TestBasicAuthConfigEmpty(c *gc.C) {
	cfg := docker.BasicAuthConfig{}
	c.Assert(cfg.Empty(), jc.DeepEquals, true)

	cfg = docker.BasicAuthConfig{
		Auth: "xxx",
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
