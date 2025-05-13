// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/proxy"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/containermanager"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
)

type suite struct {
	state          *MockState
	provider       *MockProvider
	providerGetter func(context.Context) (Provider, error)
}

var _ = tc.Suite(&suite{})

func (s *suite) SetUpTest(c *tc.C) {
	// Default provider getter function
	s.providerGetter = func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}
}

func (s *suite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.provider = NewMockProvider(ctrl)
	return ctrl
}

// TestContainerManagerConfigForType asserts the happy path for
// Service.ContainerManagerConfigForType.
func (s *suite) TestContainerManagerConfigForType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelConfigKeyValues(gomock.Any(), gomock.Any()).Return(map[string]string{
		config.LXDSnapChannel:                            "5.0/stable",
		config.ContainerImageMetadataURLKey:              "https://images.linuxcontainers.org/",
		config.ContainerImageMetadataDefaultsDisabledKey: "true",
		config.ContainerImageStreamKey:                   "released",
	}, nil)
	modelID := modeltesting.GenModelUUID(c)
	s.state.EXPECT().ModelID(gomock.Any()).Return(modelID, nil)

	service := NewService(s.state, s.providerGetter)
	cfg, err := service.ContainerManagerConfigForType(context.Background(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg, tc.DeepEquals, containermanager.Config{
		ImageMetadataURL:         "https://images.linuxcontainers.org/",
		ImageStream:              "released",
		LXDSnapChannel:           "5.0/stable",
		MetadataDefaultsDisabled: true,
		ModelID:                  modelID,
	})
}

// TestDetermineNetworkingMethodUserDefined tests that if the user specifies a
// container networking method in model config, this will be used in the
// container manager config.
func (s *suite) TestContainerNetworkingMethodUserDefined(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelConfigKeyValues(gomock.Any(), gomock.Any()).Return(map[string]string{
		config.ContainerNetworkingMethodKey: "local",
	}, nil)

	service := NewService(s.state, s.providerGetter)
	method, err := service.ContainerNetworkingMethod(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(method, tc.Equals, containermanager.NetworkingMethodLocal)
}

// TestDetermineNetworkingMethodProviderNotSupported tests the case when the
// provider does not support the Provider interface of this package.
func (s *suite) TestDetermineNetworkingMethodProviderNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelConfigKeyValues(gomock.Any(), gomock.Any()).Return(map[string]string{
		config.ContainerNetworkingMethodKey: "", // auto-configure
	}, nil)
	providerGetter := func(ctx context.Context) (Provider, error) {
		return nil, errors.Errorf("provider type %T %w", Provider(nil), coreerrors.NotSupported)
	}

	service := NewService(s.state, providerGetter)
	method, err := service.ContainerNetworkingMethod(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(method, tc.Equals, containermanager.NetworkingMethodLocal)
}

// TestDetermineNetworkingMethodProviderSupports tests that if the provider
// supports container addresses, the container networking method will be set
// to "provider".
func (s *suite) TestDetermineNetworkingMethodProviderSupports(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelConfigKeyValues(gomock.Any(), gomock.Any()).Return(map[string]string{
		config.ContainerNetworkingMethodKey: "", // auto-configure
	}, nil)
	s.provider.EXPECT().SupportsContainerAddresses(gomock.Any()).Return(true, nil)

	service := NewService(s.state, s.providerGetter)
	method, err := service.ContainerNetworkingMethod(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(method, tc.Equals, containermanager.NetworkingMethodProvider)
}

// TestDetermineNetworkingMethodProviderDoesntSupport tests that if the
// provider doesn't support container addresses, the container networking
// method will be set to "local".
func (s *suite) TestDetermineNetworkingMethodProviderDoesntSupport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelConfigKeyValues(gomock.Any(), gomock.Any()).Return(map[string]string{
		config.ContainerNetworkingMethodKey: "", // auto-configure
	}, nil)
	s.provider.EXPECT().SupportsContainerAddresses(gomock.Any()).Return(false, nil)

	service := NewService(s.state, s.providerGetter)
	method, err := service.ContainerNetworkingMethod(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(method, tc.Equals, containermanager.NetworkingMethodLocal)
}

// TestDetermineNetworkingMethodProviderDoesntSupport tests that if the
// provider returns an [errors.NotSupported] from SupportsContainerAddresses,
// the container networking method will be set to "local".
func (s *suite) TestDetermineNetworkingMethodProviderReturnsNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelConfigKeyValues(gomock.Any(), gomock.Any()).Return(map[string]string{
		config.ContainerNetworkingMethodKey: "", // auto-configure
	}, nil)
	s.provider.EXPECT().SupportsContainerAddresses(gomock.Any()).Return(false, errors.Errorf("container addresses %w", coreerrors.NotSupported))

	service := NewService(s.state, s.providerGetter)
	method, err := service.ContainerNetworkingMethod(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(method, tc.Equals, containermanager.NetworkingMethodLocal)
}

func (s *suite) TestContainerConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelConfigKeyValues(gomock.Any(), gomock.Any()).Return(map[string]string{
		config.EnableOSRefreshUpdateKey:      "true",
		config.EnableOSUpgradeKey:            "true",
		config.TypeKey:                       "ec2",
		config.SSLHostnameVerificationKey:    "true",
		config.HTTPProxyKey:                  "http://user@10.0.0.1",
		config.HTTPSProxyKey:                 "https://user@10.0.0.1",
		config.FTPProxyKey:                   "ftp://user@10.0.0.1",
		config.NoProxyKey:                    "localhost,10.0.3.1",
		config.JujuHTTPProxyKey:              "http://juju@10.0.0.1",
		config.JujuHTTPSProxyKey:             "https://juju@10.0.0.1",
		config.JujuFTPProxyKey:               "",
		config.JujuNoProxyKey:                "localhost,10.0.4.2",
		config.AptHTTPProxyKey:               "my-apt-proxy@10.0.0.2",
		config.AptHTTPSProxyKey:              "",
		config.AptFTPProxyKey:                "",
		config.AptNoProxyKey:                 "",
		config.AptMirrorKey:                  "http://my.archive.ubuntu.com",
		config.SnapHTTPProxyKey:              "http://snap-proxy",
		config.SnapHTTPSProxyKey:             "https://snap-proxy",
		config.SnapStoreAssertionsKey:        "trust us",
		config.SnapStoreProxyKey:             "42",
		config.SnapStoreProxyURLKey:          "http://snap-store-proxy",
		config.CloudInitUserDataKey:          validCloudInitUserData,
		config.ContainerInheritPropertiesKey: "ca-certs,apt-primary",
	}, nil)

	service := NewService(s.state, s.providerGetter)
	containerConfig, err := service.ContainerConfig(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(containerConfig, tc.DeepEquals, container.Config{
		EnableOSRefreshUpdate:   true,
		EnableOSUpgrade:         true,
		ProviderType:            "ec2",
		SSLHostnameVerification: true,
		LegacyProxy: proxy.Settings{
			Http:    "http://user@10.0.0.1",
			Https:   "https://user@10.0.0.1",
			Ftp:     "ftp://user@10.0.0.1",
			NoProxy: "localhost,10.0.3.1",
		},
		JujuProxy: proxy.Settings{
			Http:    "http://juju@10.0.0.1",
			Https:   "https://juju@10.0.0.1",
			Ftp:     "",
			NoProxy: "localhost,10.0.4.2",
		},
		AptProxy: proxy.Settings{
			Http:    "http://my-apt-proxy@10.0.0.2",
			Https:   "https://juju@10.0.0.1",
			Ftp:     "ftp://user@10.0.0.1",
			NoProxy: "localhost,10.0.3.1",
		},
		SnapProxy: proxy.Settings{
			Http:  "http://snap-proxy",
			Https: "https://snap-proxy",
		},
		SnapStoreAssertions:        "trust us",
		SnapStoreProxyID:           "42",
		SnapStoreProxyURL:          "http://snap-store-proxy",
		AptMirror:                  "http://my.archive.ubuntu.com",
		ContainerInheritProperties: "ca-certs,apt-primary",
		CloudInitUserData: map[string]any{
			"packages":        []any{"python-keystoneclient", "python-glanceclient"},
			"preruncmd":       []any{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
			"postruncmd":      []any{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
			"package_upgrade": false,
			"ca_certs": map[string]any{
				"trusted": []any{"root-cert", "intermediate-cert"},
			},
		},
	})
}

var validCloudInitUserData = `
packages:
  - 'python-keystoneclient'
  - 'python-glanceclient'
preruncmd:
  - mkdir /tmp/preruncmd
  - mkdir /tmp/preruncmd2
postruncmd:
  - mkdir /tmp/postruncmd
  - mkdir /tmp/postruncmd2
package_upgrade: false
ca_certs:
  trusted:
    - root-cert
    - intermediate-cert
`[1:]
