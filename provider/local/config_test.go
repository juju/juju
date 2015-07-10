// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/container/lxc"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider"
	"github.com/juju/juju/provider/local"
	"github.com/juju/juju/testing"
)

type configSuite struct {
	baseProviderSuite
}

var _ = gc.Suite(&configSuite{})

func minimalConfigValues() map[string]interface{} {
	return testing.FakeConfig().Merge(testing.Attrs{
		"name": "test",
		"type": provider.Local,
	})
}

func minimalConfig(c *gc.C) *config.Config {
	return localConfig(c, nil)
}

func localConfig(c *gc.C, extra map[string]interface{}) *config.Config {
	values := minimalConfigValues()
	for key, value := range extra {
		values[key] = value
	}
	testConfig, err := config.New(config.NoDefaults, values)
	c.Assert(err, jc.ErrorIsNil)
	testEnv, err := local.Provider.PrepareForBootstrap(envtesting.BootstrapContext(c), testConfig)
	c.Assert(err, jc.ErrorIsNil)
	return testEnv.Config()
}

func (s *configSuite) TestDefaultNetworkBridge(c *gc.C) {
	config := minimalConfig(c)
	unknownAttrs := config.UnknownAttrs()
	c.Assert(unknownAttrs["container"], gc.Equals, "lxc")
	c.Assert(unknownAttrs["network-bridge"], gc.Equals, "lxcbr0")
}

func (s *configSuite) TestDefaultNetworkBridgeForKVMContainers(c *gc.C) {
	testConfig := localConfig(c, map[string]interface{}{
		"container": "kvm",
	})
	containerType, bridgeName := local.ContainerAndBridge(c, testConfig)
	c.Check(containerType, gc.Equals, string(instance.KVM))
	c.Check(bridgeName, gc.Equals, kvm.DefaultKvmBridge)
}

func (s *configSuite) TestExplicitNetworkBridgeForLXCContainers(c *gc.C) {
	testConfig := localConfig(c, map[string]interface{}{
		"container":      "lxc",
		"network-bridge": "foo",
	})
	containerType, bridgeName := local.ContainerAndBridge(c, testConfig)
	c.Check(containerType, gc.Equals, string(instance.LXC))
	c.Check(bridgeName, gc.Equals, "foo")
}

func (s *configSuite) TestExplicitNetworkBridgeForKVMContainers(c *gc.C) {
	testConfig := localConfig(c, map[string]interface{}{
		"container":      "kvm",
		"network-bridge": "lxcbr0",
	})
	containerType, bridgeName := local.ContainerAndBridge(c, testConfig)
	c.Check(containerType, gc.Equals, string(instance.KVM))
	c.Check(bridgeName, gc.Equals, "lxcbr0")
}

func (s *configSuite) TestDefaultNetworkBridgeForLXCContainers(c *gc.C) {
	testConfig := localConfig(c, map[string]interface{}{
		"container": "lxc",
	})
	containerType, bridgeName := local.ContainerAndBridge(c, testConfig)
	c.Check(containerType, gc.Equals, string(instance.LXC))
	c.Check(bridgeName, gc.Equals, lxc.DefaultLxcBridge)
}

func (s *configSuite) TestSetNetworkBridge(c *gc.C) {
	config := localConfig(c, map[string]interface{}{
		"network-bridge": "br0",
	})
	unknownAttrs := config.UnknownAttrs()
	c.Assert(unknownAttrs["network-bridge"], gc.Equals, "br0")
	_, bridgeName := local.ContainerAndBridge(c, config)
	c.Check(bridgeName, gc.Equals, "br0")
}

func (s *configSuite) TestValidateConfig(c *gc.C) {
	valid := minimalConfig(c)
	expectedRootDir := filepath.Join(utils.Home(), ".juju", "test")
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["root-dir"], gc.Equals, expectedRootDir)
}

func (s *configSuite) TestValidateConfigWithRootDir(c *gc.C) {
	root := c.MkDir()
	valid := localConfig(c, map[string]interface{}{
		"root-dir": root,
	})
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["root-dir"], gc.Equals, root)
}

func (s *configSuite) TestValidateConfigWithTildeInRootDir(c *gc.C) {
	valid := localConfig(c, map[string]interface{}{
		"root-dir": "~/.juju/foo",
	})
	expectedRootDir := filepath.Join(utils.Home(), ".juju", "foo")
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["root-dir"], gc.Equals, expectedRootDir)
}

func (s *configSuite) TestValidateConfigWithFloatPort(c *gc.C) {
	// When the config values get serialized through JSON, the integers
	// get coerced to float64 values.  The parsing needs to handle this.
	valid := localConfig(c, map[string]interface{}{
		"storage-port": float64(8040),
	})
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["storage-port"], gc.Equals, int(8040))
}

func (s *configSuite) TestNamespace(c *gc.C) {
	s.PatchEnvironment("USER", "tester")
	testConfig := minimalConfig(c)
	c.Logf("\n\nname: %s\n\n", testConfig.Name())
	local.CheckConfigNamespace(c, testConfig, "tester-test")
}

func (s *configSuite) TestBootstrapAsRoot(c *gc.C) {
	s.PatchValue(local.CheckIfRoot, func() bool { return true })
	env, err := local.Provider.PrepareForBootstrap(envtesting.BootstrapContext(c), minimalConfig(c))
	c.Assert(err, jc.ErrorIsNil)
	_, _, _, err = env.Bootstrap(envtesting.BootstrapContext(c), environs.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "bootstrapping a local environment must not be done as root")
}

func (s *configSuite) TestLocalDisablesUpgradesWhenCloning(c *gc.C) {
	// Default config files set these to true.
	testConfig := minimalConfig(c)
	c.Check(testConfig.EnableOSRefreshUpdate(), jc.IsTrue)
	c.Check(testConfig.EnableOSUpgrade(), jc.IsTrue)

	// If using lxc-clone, we set updates to false
	validConfig := localConfig(c, map[string]interface{}{
		"lxc-clone": true,
	})
	c.Check(validConfig.EnableOSRefreshUpdate(), jc.IsTrue)
	c.Check(validConfig.EnableOSUpgrade(), jc.IsFalse)
}

// If settings are provided, don't overwrite with defaults.
func (s *configSuite) TestLocalRespectsUpgradeSettings(c *gc.C) {
	minAttrs := testing.FakeConfig().Merge(testing.Attrs{
		"lxc-clone":          true,
		"enable-os-upgrades": true,
		"enable-os-updates":  true,
	})
	testConfig, err := config.New(config.NoDefaults, minAttrs)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testConfig.EnableOSRefreshUpdate(), jc.IsTrue)
	c.Check(testConfig.EnableOSUpgrade(), jc.IsTrue)
}

func (*configSuite) TestSchema(c *gc.C) {
	fields := local.Provider.Schema()
	// Check that all the fields defined in environs/config
	// are in the returned schema.
	globalFields, err := config.Schema(nil)
	c.Assert(err, gc.IsNil)
	for name, field := range globalFields {
		c.Check(fields[name], jc.DeepEquals, field)
	}
}
