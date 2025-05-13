// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"path/filepath"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/errors"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/core/status"
	domainapplication "github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/charmdownloader"
	charmtesting "github.com/juju/juju/internal/charm/testing"
	"github.com/juju/juju/internal/testing"
)

type deployerSuite struct {
	baseSuite

	clock *MockClock
}

var _ = tc.Suite(&deployerSuite{})

func (s *deployerSuite) TestValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	err := cfg.Validate()
	c.Assert(err, tc.IsNil)

	cfg = s.newConfig(c)
	cfg.DataDir = ""
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.ObjectStore = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.ControllerConfig = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.NewCharmHubRepo = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.NewCharmDownloader = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.CharmhubHTTPClient = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Logger = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *deployerSuite) TestControllerCharmArchWithDefaultArch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	deployer := makeBaseDeployer(cfg)

	arch := deployer.ControllerCharmArch()
	c.Assert(arch, tc.Equals, "amd64")
}

func (s *deployerSuite) TestControllerCharmArch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	cfg.Constraints = constraints.Value{
		Arch: ptr("arm64"),
	}
	deployer := makeBaseDeployer(cfg)

	arch := deployer.ControllerCharmArch()
	c.Assert(arch, tc.Equals, "arm64")
}

func (s *deployerSuite) TestDeployLocalCharmThatDoesNotExist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that if the local charm doesn't exist that we get a not found
	// error. The not found error signals to the caller that they should
	// attempt the charmhub path.

	cfg := s.newConfig(c)
	deployer := makeBaseDeployer(cfg)

	_, err := deployer.DeployLocalCharm(context.Background(), arch.DefaultArchitecture, base.MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *deployerSuite) TestDeployLocalCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	archivePath, archiveSize := s.ensureControllerCharm(c, cfg.DataDir)

	hash := "a6b7b481f0e452ecf77fc65bcab7504caeb56c274c54649cd2fdaf2247b8a00b"

	s.applicationService.EXPECT().ResolveControllerCharmDownload(gomock.Any(), domainapplication.ResolveControllerCharmDownload{
		SHA256: hash,
		SHA384: "35549bf6887fe51afad516064b6b07bc429f766d464295fa632aa6040d041e11143fce549626681531fd923900a9b66c",
		Path:   archivePath,
		Size:   archiveSize,
	}).Return(domainapplication.ResolvedControllerCharmDownload{
		Charm:           s.charm,
		ArchivePath:     "path",
		ObjectStoreUUID: "1234",
	}, nil)

	deployer := s.newBaseDeployer(c, cfg)

	info, err := deployer.DeployLocalCharm(context.Background(), "arm64", base.MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.URL.String(), tc.Equals, "local:juju-controller-0")
	c.Assert(info.Origin, tc.DeepEquals, &corecharm.Origin{
		Source:   corecharm.Local,
		Type:     "charm",
		Hash:     hash,
		Revision: ptr(0),
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "ubuntu",
			Channel:      "22.04/stable",
		},
	})
	c.Assert(info.Charm, tc.NotNil)
}

func (s *deployerSuite) TestDeployCharmhubCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can deploy the charmhub charm, which by default is
	// juju-controller.

	cfg := s.newConfig(c)

	s.expectDownloadAndResolve(c, "juju-controller")

	deployer := s.newBaseDeployer(c, cfg)

	info, err := deployer.DeployCharmhubCharm(context.Background(), "arm64", base.MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.URL.String(), tc.Equals, "ch:arm64/juju-controller-1")
	c.Assert(info.Origin, tc.DeepEquals, &corecharm.Origin{
		Source:   corecharm.CharmHub,
		Type:     "charm",
		Channel:  &charm.Channel{},
		Hash:     "sha-256",
		Revision: ptr(1),
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "ubuntu",
			Channel:      "22.04",
		},
	})
	c.Assert(info.Charm, tc.NotNil)
}

func (s *deployerSuite) TestDeployCharmhubCharmWithCustomName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can deploy a charmhub charm with a custom name.

	cfg := s.newConfig(c)
	cfg.ControllerCharmName = "inferi"

	s.expectDownloadAndResolve(c, "inferi")

	deployer := s.newBaseDeployer(c, cfg)

	info, err := deployer.DeployCharmhubCharm(context.Background(), "arm64", base.MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.URL.String(), tc.Equals, "ch:arm64/inferi-1")
	c.Assert(info.Origin, tc.DeepEquals, &corecharm.Origin{
		Source:   corecharm.CharmHub,
		Type:     "charm",
		Channel:  &charm.Channel{},
		Hash:     "sha-256",
		Revision: ptr(1),
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "ubuntu",
			Channel:      "22.04",
		},
	})
	c.Assert(info.Charm, tc.NotNil)
}

func (s *deployerSuite) TestAddControllerApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that we can add the controller application to the model. This will
	// query the backend to ensure that the charm we just uploaded exists before
	// we can add the application.

	now := clock.WallClock.Now()
	s.clock.EXPECT().Now().Return(now).AnyTimes()

	cfg := s.newConfig(c)
	cfg.Clock = s.clock

	curl := "ch:juju-controller-0"

	// The application is called "controller" and the charm is called
	// "juju-controller". Do not change this, or the controller charm won't
	// come back up.

	s.applicationService.EXPECT().CreateApplication(
		gomock.Any(),
		bootstrap.ControllerApplicationName,
		s.charm,
		corecharm.Origin{
			Source:   "charm-hub",
			Type:     "charm",
			Channel:  &charm.Channel{},
			Revision: ptr(1),
			Hash:     "sha-256",
			Platform: corecharm.Platform{
				Architecture: "arm64",
				OS:           "ubuntu",
				Channel:      "22.04",
			},
		},
		applicationservice.AddApplicationArgs{
			ReferenceName: bootstrap.ControllerCharmName,
			DownloadInfo: &applicationcharm.DownloadInfo{
				CharmhubIdentifier: "abcd",
				Provenance:         applicationcharm.ProvenanceBootstrap,
				DownloadURL:        "https://inferi.com",
				DownloadSize:       42,
			},
			CharmStoragePath:     "path",
			CharmObjectStoreUUID: "1234",
			ApplicationSettings: domainapplication.ApplicationSettings{
				Trust: true,
			},
			ApplicationStatus: &status.StatusInfo{
				Status: status.Unset,
				Since:  ptr(now),
			},
		},
		applicationservice.AddUnitArg{},
	)

	deployer := s.newBaseDeployer(c, cfg)

	origin := corecharm.Origin{
		Source:   corecharm.CharmHub,
		Type:     "charm",
		Channel:  &charm.Channel{},
		Revision: ptr(1),
		Hash:     "sha-256",
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "ubuntu",
			Channel:      "22.04",
		},
	}
	address := "10.0.0.1"
	unit, err := deployer.AddControllerApplication(context.Background(), DeployCharmInfo{
		URL:    charm.MustParseURL(curl),
		Charm:  s.charm,
		Origin: &origin,
		DownloadInfo: &corecharm.DownloadInfo{
			CharmhubIdentifier: "abcd",
			DownloadURL:        "https://inferi.com",
			DownloadSize:       42,
		},
		ArchivePath:     "path",
		ObjectStoreUUID: "1234",
	}, address)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit, tc.NotNil)
}

func (s *deployerSuite) ensureControllerCharm(c *tc.C, dataDir string) (string, int64) {
	// This will place the most basic charm (no hooks, no config, no actions)
	// into the data dir so that we can test the local charm path.

	metadata := `
name: juju-controller
summary: Juju controller
description: Juju controller
`

	manifest := `
bases:
- architectures:
  - amd64
  channel: '20.04'
  name: ubuntu
- architectures:
  - amd64
  channel: '22.04'
  name: ubuntu
- architectures:
  - amd64
  channel: '24.04'
  name: ubuntu
`

	dir := c.MkDir()
	err := os.WriteFile(filepath.Join(dir, "metadata.yaml"), []byte(metadata), 0644)
	c.Assert(err, tc.ErrorIsNil)

	err = os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0644)
	c.Assert(err, tc.ErrorIsNil)

	charmDir, err := charmtesting.ReadCharmDir(dir)
	c.Assert(err, tc.ErrorIsNil)

	var buf bytes.Buffer
	charmDir.ArchiveTo(&buf)

	path := filepath.Join(dataDir, "charms")
	err = os.MkdirAll(path, 0755)
	c.Assert(err, tc.ErrorIsNil)

	path = filepath.Join(path, bootstrap.ControllerCharmArchive)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	c.Assert(err, tc.ErrorIsNil)

	defer f.Close()

	size, err := f.Write(buf.Bytes())
	c.Assert(err, tc.ErrorIsNil)
	err = f.Close()
	c.Assert(err, tc.ErrorIsNil)

	return path, int64(size)
}

func (s *deployerSuite) newBaseDeployer(c *tc.C, cfg BaseDeployerConfig) baseDeployer {
	deployer := makeBaseDeployer(cfg)

	deployer.objectStore = s.objectStore

	return deployer
}

func (s *deployerSuite) expectDownloadAndResolve(c *tc.C, name string) {
	uuid := testing.ModelTag.Id()
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":         "model",
		"type":         "type",
		"uuid":         uuid,
		"charmhub-url": "https://api.staging.charmhub.io",
	})
	c.Assert(err, tc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)

	curl := &charm.URL{
		Schema:       string(charm.CharmHub),
		Name:         name,
		Revision:     0,
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
	resolvedOrigin := corecharm.Origin{
		Source:   corecharm.CharmHub,
		Type:     "charm",
		Channel:  &charm.Channel{},
		Hash:     "sha-256",
		Revision: ptr(1),
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "ubuntu",
			Channel:      "22.04",
		},
	}

	s.charmRepo.EXPECT().ResolveWithPreferredChannel(gomock.Any(), name, origin).Return(corecharm.ResolvedData{
		URL:    curl,
		Origin: resolvedOrigin,
		EssentialMetadata: corecharm.EssentialMetadata{
			ResolvedOrigin: resolvedOrigin,
			DownloadInfo: corecharm.DownloadInfo{
				DownloadURL: "https://inferi.com",
			},
		},
	}, nil)

	url, err := url.Parse("https://inferi.com")
	c.Assert(err, tc.ErrorIsNil)

	s.charmDownloader.EXPECT().Download(gomock.Any(), url, "sha-256").Return(&charmdownloader.DownloadResult{
		SHA256: "sha-256",
		SHA384: "sha-384",
		Path:   "path",
		Size:   42,
	}, nil)

	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	s.applicationService.EXPECT().ResolveControllerCharmDownload(gomock.Any(), domainapplication.ResolveControllerCharmDownload{
		SHA256: "sha-256",
		SHA384: "sha-384",
		Path:   "path",
		Size:   42,
	}).Return(domainapplication.ResolvedControllerCharmDownload{
		Charm:           s.charm,
		ArchivePath:     "path",
		ObjectStoreUUID: objectStoreUUID,
	}, nil)
}

func (s *deployerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.clock = NewMockClock(ctrl)

	return ctrl
}
