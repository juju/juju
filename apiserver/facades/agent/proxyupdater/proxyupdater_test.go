// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	"time"

	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/proxyupdater"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type ProxyUpdaterSuite struct {
	coretesting.BaseSuite

	state      *stubBackend
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	facade     *proxyupdater.APIv2
	tag        names.MachineTag
}

var _ = gc.Suite(&ProxyUpdaterSuite{})

func (s *ProxyUpdaterSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *ProxyUpdaterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("1"),
		Controller: false,
	}
	s.tag = names.NewMachineTag("1")
	s.state = &stubBackend{}
	s.state.SetUp(c)
	s.AddCleanup(func(_ *gc.C) { s.state.Kill() })

	api, err := proxyupdater.NewAPIBase(s.state, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(api, gc.NotNil)
	s.facade = &proxyupdater.APIv2{api}

	// Shouldn't have any calls yet
	apiservertesting.CheckMethodCalls(c, s.state.Stub)
}

func (s *ProxyUpdaterSuite) TestWatchForProxyConfigAndAPIHostPortChanges(c *gc.C) {
	// WatchForProxyConfigAndAPIHostPortChanges combines WatchForModelConfigChanges
	// and WatchAPIHostPorts. Check that they are both called and we get the
	result := s.facade.WatchForProxyConfigAndAPIHostPortChanges(s.oneEntity())
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	s.state.Stub.CheckCallNames(c,
		"WatchForModelConfigChanges",
		"WatchAPIHostPortsForAgents",
	)

	// Verify the watcher resource was registered.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get(result.Results[0].NotifyWatcherId)
	watcher, ok := resource.(state.NotifyWatcher)
	c.Assert(ok, jc.IsTrue)

	// Verify the initial event was consumed.
	select {
	case <-watcher.Changes():
		c.Fatalf("initial event never consumed")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *ProxyUpdaterSuite) oneEntity() params.Entities {
	entities := params.Entities{
		make([]params.Entity, 1),
	}
	entities.Entities[0].Tag = s.tag.String()
	return entities
}

func (s *ProxyUpdaterSuite) TestProxyConfigV1(c *gc.C) {
	// Check that the ProxyConfig combines data from ModelConfig and APIHostPorts
	v1 := &proxyupdater.APIv1{s.facade}
	cfg := v1.ProxyConfig(s.oneEntity())

	s.state.Stub.CheckCallNames(c,
		"ModelConfig",
		"APIHostPortsForAgents",
	)

	noProxy := "0.1.2.3,0.1.2.4,0.1.2.5"

	r := params.ProxyConfigResultV1{
		ProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy", FTP: "", NoProxy: noProxy},
		APTProxySettings: params.ProxyConfig{
			HTTP: "http://apt http proxy", HTTPS: "https://apt https proxy", FTP: "", NoProxy: ""},
	}
	c.Assert(cfg.Results[0], jc.DeepEquals, r)
}

func (s *ProxyUpdaterSuite) TestProxyConfig(c *gc.C) {
	// Check that the ProxyConfig combines data from ModelConfig and APIHostPorts
	cfg := s.facade.ProxyConfig(s.oneEntity())

	s.state.Stub.CheckCallNames(c,
		"ModelConfig",
		"APIHostPortsForAgents",
	)

	expectedLegacyNoProxy := "0.1.2.3,0.1.2.4,0.1.2.5"
	expectedJujuNoProxy := ""

	r := params.ProxyConfigResult{
		LegacyProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy", FTP: "", NoProxy: expectedLegacyNoProxy},
		JujuProxySettings: params.ProxyConfig{
			HTTP: "", HTTPS: "", FTP: "", NoProxy: expectedJujuNoProxy},
		APTProxySettings: params.ProxyConfig{
			HTTP: "http://apt http proxy", HTTPS: "https://apt https proxy", FTP: "", NoProxy: ""},
	}
	c.Assert(cfg.Results[0], jc.DeepEquals, r)
}

func (s *ProxyUpdaterSuite) TestProxyConfigJujuProxy(c *gc.C) {
	s.state.SetModelConfig(coretesting.Attrs{
		"juju-http-proxy":  "http proxy",
		"juju-https-proxy": "https proxy",
		"apt-http-proxy":   "apt http proxy",
		"apt-https-proxy":  "apt https proxy",
	})

	cfg := s.facade.ProxyConfig(s.oneEntity())

	s.state.Stub.CheckCallNames(c,
		"ModelConfig",
		"APIHostPortsForAgents",
	)

	// need to make sure that auto-population/auto-appending of controller IPs to
	// no-proxy is aware of which proxy settings are used: if non-legacy ones are used
	// then juju-no-proxy should be auto-modified
	expectedJujuNoProxy := "0.1.2.3,0.1.2.4,0.1.2.5"
	expectedLegacyNoProxy := ""

	r := params.ProxyConfigResult{
		JujuProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy", FTP: "", NoProxy: expectedJujuNoProxy},
		LegacyProxySettings: params.ProxyConfig{
			HTTP: "", HTTPS: "", FTP: "", NoProxy: expectedLegacyNoProxy},
		APTProxySettings: params.ProxyConfig{
			HTTP: "http://apt http proxy", HTTPS: "https://apt https proxy", FTP: "", NoProxy: ""},
	}
	c.Assert(cfg.Results[0], jc.DeepEquals, r)
}

func (s *ProxyUpdaterSuite) TestProxyConfigExtendsExisting(c *gc.C) {
	// Check that the ProxyConfig combines data from ModelConfig and APIHostPorts
	s.state.SetModelConfig(coretesting.Attrs{
		"http-proxy":      "http proxy",
		"https-proxy":     "https proxy",
		"apt-http-proxy":  "apt http proxy",
		"apt-https-proxy": "apt https proxy",
		"no-proxy":        "9.9.9.9",
	})
	cfg := s.facade.ProxyConfig(s.oneEntity())
	s.state.Stub.CheckCallNames(c,
		"ModelConfig",
		"APIHostPortsForAgents",
	)

	expectedNoProxy := "0.1.2.3,0.1.2.4,0.1.2.5,9.9.9.9"
	expectedAptNoProxy := "9.9.9.9"

	c.Assert(cfg.Results[0], jc.DeepEquals, params.ProxyConfigResult{
		LegacyProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy", FTP: "", NoProxy: expectedNoProxy},
		APTProxySettings: params.ProxyConfig{
			HTTP: "http://apt http proxy", HTTPS: "https://apt https proxy", FTP: "", NoProxy: expectedAptNoProxy},
	})
}

func (s *ProxyUpdaterSuite) TestProxyConfigNoDuplicates(c *gc.C) {
	// Check that the ProxyConfig combines data from ModelConfig and APIHostPorts
	s.state.SetModelConfig(coretesting.Attrs{
		"http-proxy":      "http proxy",
		"https-proxy":     "https proxy",
		"apt-http-proxy":  "apt http proxy",
		"apt-https-proxy": "apt https proxy",
		"no-proxy":        "0.1.2.3",
	})
	cfg := s.facade.ProxyConfig(s.oneEntity())
	s.state.Stub.CheckCallNames(c,
		"ModelConfig",
		"APIHostPortsForAgents",
	)

	expectedNoProxy := "0.1.2.3,0.1.2.4,0.1.2.5"
	expectedAptNoProxy := "0.1.2.3"

	c.Assert(cfg.Results[0], jc.DeepEquals, params.ProxyConfigResult{
		LegacyProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy", FTP: "", NoProxy: expectedNoProxy},
		APTProxySettings: params.ProxyConfig{
			HTTP: "http://apt http proxy", HTTPS: "https://apt https proxy", FTP: "", NoProxy: expectedAptNoProxy},
	})
}

func (s *ProxyUpdaterSuite) TestSnapProxyConfig(c *gc.C) {
	s.state.SetModelConfig(coretesting.Attrs{
		"snap-http-proxy":       "http proxy",
		"snap-https-proxy":      "https proxy",
		"snap-store-proxy":      "store proxy",
		"snap-store-assertions": "trust us",
	})
	cfg := s.facade.ProxyConfig(s.oneEntity())
	s.state.Stub.CheckCallNames(c,
		"ModelConfig",
		"APIHostPortsForAgents",
	)

	expectedNoProxy := "0.1.2.3,0.1.2.4,0.1.2.5"

	c.Assert(cfg.Results[0], jc.DeepEquals, params.ProxyConfigResult{
		LegacyProxySettings: params.ProxyConfig{NoProxy: expectedNoProxy},
		SnapProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy"},
		SnapStoreProxyId:         "store proxy",
		SnapStoreProxyAssertions: "trust us",
	})
}

type stubBackend struct {
	*testing.Stub

	EnvConfig   *config.Config
	c           *gc.C
	configAttrs coretesting.Attrs
	hpWatcher   workertest.NotAWatcher
	confWatcher workertest.NotAWatcher
}

func (sb *stubBackend) SetUp(c *gc.C) {
	sb.Stub = &testing.Stub{}
	sb.c = c
	sb.configAttrs = coretesting.Attrs{
		"http-proxy":      "http proxy",
		"https-proxy":     "https proxy",
		"apt-http-proxy":  "apt http proxy",
		"apt-https-proxy": "apt https proxy",
	}
	sb.hpWatcher = workertest.NewFakeWatcher(1, 1)
	sb.confWatcher = workertest.NewFakeWatcher(1, 1)
}

func (sb *stubBackend) Kill() {
	sb.hpWatcher.Kill()
	sb.confWatcher.Kill()
}

func (sb *stubBackend) SetModelConfig(ca coretesting.Attrs) {
	sb.configAttrs = ca
}

func (sb *stubBackend) ModelConfig() (*config.Config, error) {
	sb.MethodCall(sb, "ModelConfig")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	return coretesting.CustomModelConfig(sb.c, sb.configAttrs), nil
}

func (sb *stubBackend) APIHostPortsForAgents() ([]network.SpaceHostPorts, error) {
	sb.MethodCall(sb, "APIHostPortsForAgents")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	hps := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
		network.NewSpaceHostPorts(1234, "0.1.2.4"),
		network.NewSpaceHostPorts(1234, "0.1.2.5"),
	}
	return hps, nil
}

func (sb *stubBackend) WatchAPIHostPortsForAgents() state.NotifyWatcher {
	sb.MethodCall(sb, "WatchAPIHostPortsForAgents")
	return sb.hpWatcher
}

func (sb *stubBackend) WatchForModelConfigChanges() state.NotifyWatcher {
	sb.MethodCall(sb, "WatchForModelConfigChanges")
	return sb.confWatcher
}
