// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	services "github.com/juju/juju/apiserver/facades/client/charms/services"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/bootstrap"
	state "github.com/juju/juju/state"
)

type deployerSuite struct {
	baseSuite
}

var _ = gc.Suite(&deployerSuite{})

func (s *deployerSuite) TestValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	err := cfg.Validate()
	c.Assert(err, gc.IsNil)

	cfg = s.newConfig(c)
	cfg.DataDir = ""
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.State = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.ObjectStore = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.ControllerConfig = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.NewCharmRepo = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.NewCharmDownloader = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.CharmhubHTTPClient = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.LoggerFactory = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *deployerSuite) TestControllerCharmArchWithDefaultArch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	deployer := makeBaseDeployer(cfg)

	arch := deployer.ControllerCharmArch()
	c.Assert(arch, gc.Equals, "amd64")
}

func (s *deployerSuite) TestControllerCharmArch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	cfg.Constraints = constraints.Value{
		Arch: ptr("arm64"),
	}
	deployer := makeBaseDeployer(cfg)

	arch := deployer.ControllerCharmArch()
	c.Assert(arch, gc.Equals, "arm64")
}

func (s *deployerSuite) TestDeployLocalCharmThatDoesNotExist(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that if the local charm doesn't exist that we get a not found
	// error. The not found error signals to the caller that they should
	// attempt the charmhub path.

	cfg := s.newConfig(c)
	deployer := makeBaseDeployer(cfg)

	_, _, err := deployer.DeployLocalCharm(context.Background(), arch.DefaultArchitecture, base.MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *deployerSuite) TestDeployLocalCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// When deploying the local charm, ensure that we get the returned charm URL
	// and the origin with the correct architecture and `/stable` suffix
	// on the origin channel.

	cfg := s.newConfig(c)
	s.ensureControllerCharm(c, cfg.DataDir)

	s.expectLocalCharmUpload(c)

	deployer := s.newBaseDeployer(c, cfg)

	url, origin, err := deployer.DeployLocalCharm(context.Background(), "arm64", base.MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url.String(), gc.Equals, "local:arm64/jammy/juju-controller-0")
	c.Assert(origin, gc.DeepEquals, &corecharm.Origin{
		Source: corecharm.Local,
		Type:   "charm",
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "ubuntu",
			Channel:      "22.04/stable",
		},
	})
}

func (s *deployerSuite) TestDeployCharmhubCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)

	s.expectCharmhubCharmUpload(c)

	deployer := s.newBaseDeployer(c, cfg)

	url, origin, err := deployer.DeployCharmhubCharm(context.Background(), "arm64", base.MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.Equals, "ch:arm64/jammy/juju-controller-0")
	c.Assert(origin, gc.DeepEquals, &corecharm.Origin{
		Source:  corecharm.CharmHub,
		Type:    "charm",
		Channel: &charm.Channel{},
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "ubuntu",
			Channel:      "22.04",
		},
	})
}

func (s *deployerSuite) ensureControllerCharm(c *gc.C, dataDir string) {
	metadata := `
name: juju-controller
summary: Juju controller
description: Juju controller
`
	dir := c.MkDir()
	err := os.WriteFile(filepath.Join(dir, "metadata.yaml"), []byte(metadata), 0644)
	c.Assert(err, jc.ErrorIsNil)

	charmDir, err := charm.ReadCharmDir(dir)
	c.Assert(err, jc.ErrorIsNil)

	var buf bytes.Buffer
	charmDir.ArchiveTo(&buf)

	path := filepath.Join(dataDir, "charms")
	err = os.MkdirAll(path, 0755)
	c.Assert(err, jc.ErrorIsNil)

	path = filepath.Join(path, bootstrap.ControllerCharmArchive)
	err = os.WriteFile(path, buf.Bytes(), 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *deployerSuite) newBaseDeployer(c *gc.C, cfg BaseDeployerConfig) baseDeployer {
	deployer := makeBaseDeployer(cfg)

	deployer.stateBackend = s.stateBackend
	deployer.charmUploader = s.charmUploader
	deployer.objectStore = s.objectStore

	return deployer
}

func (s *deployerSuite) expectLocalCharmUpload(c *gc.C) {
	s.charmUploader.EXPECT().PrepareLocalCharmUpload("local:jammy/juju-controller-0").Return(&charm.URL{
		Schema:       "local",
		Name:         "juju-controller",
		Revision:     0,
		Series:       "jammy",
		Architecture: "arm64",
	}, nil)
	// Ensure that the charm uploaded to the object store is the one we expect.
	s.objectStore.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, path string, reader io.Reader, size int64) error {
		c.Check(strings.HasPrefix(path, "charms/local:arm64/jammy/juju-controller-0"), jc.IsTrue)
		return nil
	})
	s.charmUploader.EXPECT().UpdateUploadedCharm(gomock.Any()).DoAndReturn(func(info state.CharmInfo) (services.UploadedCharm, error) {
		c.Check(info.ID, gc.Equals, "local:arm64/jammy/juju-controller-0")
		c.Check(strings.HasPrefix(info.StoragePath, "charms/local:arm64/jammy/juju-controller-0"), jc.IsTrue)

		return nil, nil
	})
}

func (s *deployerSuite) expectCharmhubCharmUpload(c *gc.C) {
	curl := &charm.URL{
		Schema:       string(charm.CharmHub),
		Name:         "juju-controller",
		Revision:     0,
		Series:       "jammy",
		Architecture: "arm64",
	}
	origin := corecharm.Origin{
		Source:  corecharm.CharmHub,
		Type:    "charm",
		Channel: &charm.Channel{},
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "ubuntu",
			Channel:      "22.04",
		},
	}

	s.stateBackend.EXPECT().Model().Return(s.model, nil)
	s.charmRepo.EXPECT().ResolveWithPreferredChannel(gomock.Any(), "juju-controller", origin).Return(curl, origin, nil, nil)
	s.charmDownloader.EXPECT().DownloadAndStore(gomock.Any(), curl, origin, false).Return(origin, nil)
}
