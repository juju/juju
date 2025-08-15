// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	"sync"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/proxyupdater"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type ProxyUpdaterSuite struct {
	coretesting.BaseSuite

	authorizer apiservertesting.FakeAuthorizer
	facade     *proxyupdater.API
	tag        names.MachineTag

	modelConfigService    *MockModelConfigService
	controllerNodeService *MockControllerNodeService
	watcherRegistry       *facademocks.MockWatcherRegistry
}

func TestProxyUpdaterSuite(t *testing.T) {
	tc.Run(t, &ProxyUpdaterSuite{})
}

func (s *ProxyUpdaterSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *ProxyUpdaterSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("1"),
		Controller: false,
	}
	s.tag = names.NewMachineTag("1")
}

func (s *ProxyUpdaterSuite) setupAPI(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	api, err := proxyupdater.NewAPIV2(s.controllerNodeService, s.modelConfigService, s.authorizer, s.watcherRegistry)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(api, tc.NotNil)
	s.facade = api

	c.Cleanup(func() {
		s.facade = nil
		s.controllerNodeService = nil
		s.modelConfigService = nil
		s.watcherRegistry = nil
	})

	return ctrl
}

func (s *ProxyUpdaterSuite) TestWatchForProxyConfigAndAPIHostPortChanges(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()
	done := make(chan struct{})
	defer close(done)
	wg := sync.WaitGroup{}
	defer wg.Wait()

	// WatchForProxyConfigAndAPIHostPortChanges combines WatchForModelConfigChanges
	// and WatchAPIHostPorts. Check that they are both called.

	// Create fake model config watcher preloaded with one item in the channel
	modelConfigChanges := make(chan []string, 1)
	modelConfigWatcher := watchertest.NewMockStringsWatcher(modelConfigChanges)
	modelConfigChanges <- []string{}
	s.modelConfigService.EXPECT().Watch(gomock.Any()).Return(modelConfigWatcher, nil)

	apiHostPortsForAgentsChanged := make(chan struct{}, 1)
	hostPortWatcher := watchertest.NewMockNotifyWatcher(apiHostPortsForAgentsChanged)
	apiHostPortsForAgentsChanged <- struct{}{}
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(hostPortWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("42", nil)

	result := s.facade.WatchForProxyConfigAndAPIHostPortChanges(c.Context(), s.oneEntity())
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].NotifyWatcherId, tc.Equals, "42")
}

func (s *ProxyUpdaterSuite) oneEntity() params.Entities {
	entities := params.Entities{
		Entities: make([]params.Entity, 1),
	}
	entities.Entities[0].Tag = s.tag.String()
	return entities
}

func (s *ProxyUpdaterSuite) TestMirrorConfig(c *tc.C) {
	s.setupAPI(c).Finish()
	// Arrange
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(coretesting.CustomModelConfig(c,
		coretesting.Attrs{
			"apt-mirror": "http://mirror",
		},
	), nil)
	s.controllerNodeService.EXPECT().GetAllNoProxyAPIAddressesForAgents(gomock.Any()).Return("", nil)

	// Act
	cfg := s.facade.ProxyConfig(c.Context(), s.oneEntity())

	// Assert
	c.Assert(cfg.Results, tc.HasLen, 1)
	c.Assert(cfg.Results[0].AptMirror, tc.Equals, "http://mirror")
}

func (s *ProxyUpdaterSuite) TestProxyConfig(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	// Check that the ProxyConfig combines data from ModelConfig and APIHostPorts
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(coretesting.CustomModelConfig(c,
		coretesting.Attrs{
			"http-proxy":      "http proxy",
			"https-proxy":     "https proxy",
			"apt-http-proxy":  "apt http proxy",
			"apt-https-proxy": "apt https proxy",
		},
	), nil)
	addrs := "0.1.2.3,0.1.2.4,0.1.2.5"
	s.controllerNodeService.EXPECT().GetAllNoProxyAPIAddressesForAgents(gomock.Any()).Return(addrs, nil)

	cfg := s.facade.ProxyConfig(c.Context(), s.oneEntity())

	expectedLegacyNoProxy := addrs
	expectedJujuNoProxy := ""

	r := params.ProxyConfigResult{
		LegacyProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy", FTP: "", NoProxy: expectedLegacyNoProxy},
		JujuProxySettings: params.ProxyConfig{
			HTTP: "", HTTPS: "", FTP: "", NoProxy: expectedJujuNoProxy},
		APTProxySettings: params.ProxyConfig{
			HTTP: "http://apt http proxy", HTTPS: "https://apt https proxy", FTP: "", NoProxy: ""},
	}
	c.Assert(cfg.Results[0], tc.DeepEquals, r)
}

func (s *ProxyUpdaterSuite) TestProxyConfigJujuProxy(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(coretesting.CustomModelConfig(c,
		coretesting.Attrs{
			"juju-http-proxy":  "http proxy",
			"juju-https-proxy": "https proxy",
			"apt-http-proxy":   "apt http proxy",
			"apt-https-proxy":  "apt https proxy",
		},
	), nil)
	addrs := "0.1.2.3,0.1.2.4,0.1.2.5"
	s.controllerNodeService.EXPECT().GetAllNoProxyAPIAddressesForAgents(gomock.Any()).Return(addrs, nil)

	cfg := s.facade.ProxyConfig(c.Context(), s.oneEntity())

	// need to make sure that auto-population/auto-appending of controller IPs to
	// no-proxy is aware of which proxy settings are used: if non-legacy ones are used
	// then juju-no-proxy should be auto-modified
	expectedJujuNoProxy := addrs
	expectedLegacyNoProxy := ""

	r := params.ProxyConfigResult{
		JujuProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy", FTP: "", NoProxy: expectedJujuNoProxy},
		LegacyProxySettings: params.ProxyConfig{
			HTTP: "", HTTPS: "", FTP: "", NoProxy: expectedLegacyNoProxy},
		APTProxySettings: params.ProxyConfig{
			HTTP: "http://apt http proxy", HTTPS: "https://apt https proxy", FTP: "", NoProxy: ""},
	}
	c.Assert(cfg.Results[0], tc.DeepEquals, r)
}

func (s *ProxyUpdaterSuite) TestProxyConfigExtendsExisting(c *tc.C) {
	s.setupAPI(c).Finish()

	// Arrange
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(coretesting.CustomModelConfig(c,
		coretesting.Attrs{
			"http-proxy":      "http proxy",
			"https-proxy":     "https proxy",
			"apt-http-proxy":  "apt http proxy",
			"apt-https-proxy": "apt https proxy",
			"no-proxy":        "9.9.9.9",
		},
	), nil)
	addrs := "0.1.2.3,0.1.2.4,0.1.2.5"
	s.controllerNodeService.EXPECT().GetAllNoProxyAPIAddressesForAgents(gomock.Any()).Return(addrs, nil)

	// Act
	cfg := s.facade.ProxyConfig(c.Context(), s.oneEntity())

	// Assert
	expectedNoProxy := "0.1.2.3,0.1.2.4,0.1.2.5,9.9.9.9"
	expectedAptNoProxy := "9.9.9.9"

	c.Assert(cfg.Results[0], tc.DeepEquals, params.ProxyConfigResult{
		LegacyProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy", FTP: "", NoProxy: expectedNoProxy},
		APTProxySettings: params.ProxyConfig{
			HTTP: "http://apt http proxy", HTTPS: "https://apt https proxy", FTP: "", NoProxy: expectedAptNoProxy},
	})
}

func (s *ProxyUpdaterSuite) TestProxyConfigNoDuplicates(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	// Check that the ProxyConfig combines data from ModelConfig and APIHostPorts
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(coretesting.CustomModelConfig(c,
		coretesting.Attrs{
			"http-proxy":      "http proxy",
			"https-proxy":     "https proxy",
			"apt-http-proxy":  "apt http proxy",
			"apt-https-proxy": "apt https proxy",
			"no-proxy":        "0.1.2.3",
		},
	), nil)
	addrs := "0.1.2.3,0.1.2.4,0.1.2.5"
	s.controllerNodeService.EXPECT().GetAllNoProxyAPIAddressesForAgents(gomock.Any()).Return(addrs, nil)

	cfg := s.facade.ProxyConfig(c.Context(), s.oneEntity())

	expectedNoProxy := "0.1.2.3,0.1.2.4,0.1.2.5"
	expectedAptNoProxy := "0.1.2.3"

	c.Assert(cfg.Results[0], tc.DeepEquals, params.ProxyConfigResult{
		LegacyProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy", FTP: "", NoProxy: expectedNoProxy},
		APTProxySettings: params.ProxyConfig{
			HTTP: "http://apt http proxy", HTTPS: "https://apt https proxy", FTP: "", NoProxy: expectedAptNoProxy},
	})
}

func (s *ProxyUpdaterSuite) TestSnapProxyConfig(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(coretesting.CustomModelConfig(c,
		coretesting.Attrs{
			"snap-http-proxy":       "http proxy",
			"snap-https-proxy":      "https proxy",
			"snap-store-proxy":      "store proxy",
			"snap-store-assertions": "trust us",
		},
	), nil)
	addrs := "0.1.2.3,0.1.2.4,0.1.2.5"
	s.controllerNodeService.EXPECT().GetAllNoProxyAPIAddressesForAgents(gomock.Any()).Return(addrs, nil)

	cfg := s.facade.ProxyConfig(c.Context(), s.oneEntity())

	expectedNoProxy := addrs

	c.Assert(cfg.Results[0], tc.DeepEquals, params.ProxyConfigResult{
		LegacyProxySettings: params.ProxyConfig{NoProxy: expectedNoProxy},
		SnapProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy"},
		SnapStoreProxyId:         "store proxy",
		SnapStoreProxyAssertions: "trust us",
	})
}
