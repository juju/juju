// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"errors"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/testing"
)

type ProxyUpdaterSuite struct {
	coretesting.BaseSuite
	apiservertesting.StubNetwork

	state      *stubBackend
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	facade     API
}

var _ = gc.Suite(&ProxyUpdaterSuite{})

func (s *ProxyUpdaterSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *ProxyUpdaterSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *ProxyUpdaterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("1"),
		EnvironManager: false,
	}
	s.state = &stubBackend{}
	s.state.SetUp(c)
	s.AddCleanup(func(_ *gc.C) { s.state.Kill() })

	var err error
	s.facade, err = newAPIWithBacking(s.state, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.facade, gc.NotNil)

	// Shouldn't have any calls yet
	apiservertesting.CheckMethodCalls(c, s.state.Stub)
}

func (s *ProxyUpdaterSuite) TestWatchForProxyConfigAndAPIHostPortChanges(c *gc.C) {
	// WatchForProxyConfigAndAPIHostPortChanges combines WatchForModelConfigChanges
	// and WatchAPIHostPorts. Check that they are both called and we get the
	// expected result.
	wr, err := s.facade.WatchForProxyConfigAndAPIHostPortChanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wr, jc.DeepEquals, params.NotifyWatchResult{NotifyWatcherId: "1"})

	s.state.Stub.CheckCallNames(c,
		"WatchForModelConfigChanges",
		"WatchAPIHostPorts",
	)

}

func (s *ProxyUpdaterSuite) TestProxyConfig(c *gc.C) {
	// Check that the ProxyConfig combines data from EnvironConfig and APIHostPorts
	cfg, err := s.facade.ProxyConfig()
	c.Assert(err, jc.ErrorIsNil)
	s.state.Stub.CheckCallNames(c,
		"EnvironConfig",
		"APIHostPorts",
	)

	noProxy := "0.1.2.3,0.1.2.4,0.1.2.5"

	c.Assert(cfg, jc.DeepEquals, params.ProxyConfigResult{
		ProxySettings: proxy.Settings{
			Http: "http proxy", Https: "https proxy", Ftp: "", NoProxy: noProxy},
		APTProxySettings: proxy.Settings{
			Http: "http://http proxy", Https: "https://https proxy", Ftp: "", NoProxy: ""},
	})
}

func (s *ProxyUpdaterSuite) TestProxyConfigExtendsExisting(c *gc.C) {
	// Check that the ProxyConfig combines data from EnvironConfig and APIHostPorts
	s.state.SetEnvironConfig(coretesting.Attrs{
		"http-proxy":  "http proxy",
		"https-proxy": "https proxy",
		"no-proxy":    "9.9.9.9",
	})
	cfg, err := s.facade.ProxyConfig()
	c.Assert(err, jc.ErrorIsNil)
	s.state.Stub.CheckCallNames(c,
		"EnvironConfig",
		"APIHostPorts",
	)

	expectedNoProxy := "0.1.2.3,0.1.2.4,0.1.2.5,9.9.9.9"

	c.Assert(cfg, jc.DeepEquals, params.ProxyConfigResult{
		ProxySettings: proxy.Settings{
			Http: "http proxy", Https: "https proxy", Ftp: "", NoProxy: expectedNoProxy},
		APTProxySettings: proxy.Settings{
			Http: "http://http proxy", Https: "https://https proxy", Ftp: "", NoProxy: ""},
	})
}

func (s *ProxyUpdaterSuite) TestProxyConfigNoDuplicates(c *gc.C) {
	// Check that the ProxyConfig combines data from EnvironConfig and APIHostPorts
	s.state.SetEnvironConfig(coretesting.Attrs{
		"http-proxy":  "http proxy",
		"https-proxy": "https proxy",
		"no-proxy":    "0.1.2.3",
	})
	cfg, err := s.facade.ProxyConfig()
	c.Assert(err, jc.ErrorIsNil)
	s.state.Stub.CheckCallNames(c,
		"EnvironConfig",
		"APIHostPorts",
	)

	expectedNoProxy := "0.1.2.3,0.1.2.4,0.1.2.5"

	c.Assert(cfg, jc.DeepEquals, params.ProxyConfigResult{
		ProxySettings: proxy.Settings{
			Http: "http proxy", Https: "https proxy", Ftp: "", NoProxy: expectedNoProxy},
		APTProxySettings: proxy.Settings{
			Http: "http://http proxy", Https: "https://https proxy", Ftp: "", NoProxy: ""},
	})
}

type stubBackend struct {
	*testing.Stub

	EnvConfig   *config.Config
	c           *gc.C
	configAttrs coretesting.Attrs
	hpWatcher   notAWatcher
	confWatcher notAWatcher
}

func (sb *stubBackend) SetUp(c *gc.C) {
	sb.Stub = &testing.Stub{}
	sb.c = c
	sb.configAttrs = coretesting.Attrs{
		"http-proxy":  "http proxy",
		"https-proxy": "https proxy",
	}
	sb.hpWatcher = newFakeWatcher()
	sb.confWatcher = newFakeWatcher()
}

func (sb *stubBackend) Kill() {
	sb.hpWatcher.Kill()
	sb.confWatcher.Kill()
}

func (sb *stubBackend) SetEnvironConfig(ca coretesting.Attrs) {
	sb.configAttrs = ca
}

func (sb *stubBackend) EnvironConfig() (*config.Config, error) {
	sb.MethodCall(sb, "EnvironConfig")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	return coretesting.CustomModelConfig(sb.c, sb.configAttrs), nil
}

func (sb *stubBackend) APIHostPorts() ([][]network.HostPort, error) {
	sb.MethodCall(sb, "APIHostPorts")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	hps := [][]network.HostPort{
		network.NewHostPorts(1234, "0.1.2.3"),
		network.NewHostPorts(1234, "0.1.2.4"),
		network.NewHostPorts(1234, "0.1.2.5"),
	}
	return hps, nil
}

func (sb *stubBackend) WatchAPIHostPorts() state.NotifyWatcher {
	sb.MethodCall(sb, "WatchAPIHostPorts")
	return sb.hpWatcher
}

func (sb *stubBackend) WatchForModelConfigChanges() state.NotifyWatcher {
	sb.MethodCall(sb, "WatchForModelConfigChanges")
	return sb.confWatcher
}

type notAWatcher struct {
	changes chan struct{}
	die     chan struct{}
}

func newFakeWatcher() notAWatcher {
	ch := make(chan struct{}, 2)
	ch <- struct{}{}
	ch <- struct{}{}
	return notAWatcher{
		changes: ch,
		die:     make(chan struct{}),
	}
}

func (w notAWatcher) Changes() <-chan struct{} {
	return w.changes
}

func (w notAWatcher) Stop() error {
	return nil
}

func (w notAWatcher) Err() error {
	return errors.New("An error")
}

func (w notAWatcher) Kill() {
	select {
	case <-w.die: // already closed (don't close a closed channel)
	default:
		close(w.die)
	}
}

func (w notAWatcher) Wait() error {
	<-w.die // Wait until Kill is called.
	return nil
}
