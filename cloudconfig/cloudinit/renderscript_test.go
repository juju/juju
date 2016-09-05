// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"regexp"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/packaging"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/multiwatcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

type configureSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&configureSuite{})

type testProvider struct {
	environs.EnvironProvider
}

func init() {
	environs.RegisterProvider("sshinit_test", &testProvider{})
}

func testConfig(c *gc.C, controller bool, vers version.Binary) *config.Config {
	testConfig, err := config.New(config.UseDefaults, coretesting.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)
	testConfig, err = testConfig.Apply(map[string]interface{}{
		"type":           "sshinit_test",
		"default-series": vers.Series,
		"agent-version":  vers.Number.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return testConfig
}

func (s *configureSuite) getCloudConfig(c *gc.C, controller bool, vers version.Binary) cloudinit.CloudConfig {
	var icfg *instancecfg.InstanceConfig
	var err error
	modelConfig := testConfig(c, controller, vers)
	if controller {
		icfg, err = instancecfg.NewBootstrapInstanceConfig(
			coretesting.FakeControllerConfig(),
			constraints.Value{}, constraints.Value{},
			vers.Series, "",
		)
		c.Assert(err, jc.ErrorIsNil)
		icfg.APIInfo = &api.Info{
			Password: "password",
			CACert:   coretesting.CACert,
			ModelTag: coretesting.ModelTag,
		}
		icfg.Controller.MongoInfo = &mongo.MongoInfo{
			Password: "password", Info: mongo.Info{CACert: coretesting.CACert},
		}
		icfg.Bootstrap.ControllerModelConfig = modelConfig
		icfg.Bootstrap.BootstrapMachineInstanceId = "instance-id"
		icfg.Bootstrap.HostedModelConfig = map[string]interface{}{
			"name": "hosted-model",
		}
		icfg.Bootstrap.StateServingInfo = params.StateServingInfo{
			Cert:         coretesting.ServerCert,
			PrivateKey:   coretesting.ServerKey,
			CAPrivateKey: coretesting.CAKey,
			StatePort:    123,
			APIPort:      456,
		}
		icfg.Jobs = []multiwatcher.MachineJob{multiwatcher.JobManageModel, multiwatcher.JobHostUnits}
		icfg.Bootstrap.StateServingInfo = params.StateServingInfo{
			Cert:         coretesting.ServerCert,
			PrivateKey:   coretesting.ServerKey,
			CAPrivateKey: coretesting.CAKey,
			StatePort:    123,
			APIPort:      456,
		}
	} else {
		icfg, err = instancecfg.NewInstanceConfig(coretesting.ControllerTag, "0", "ya", imagemetadata.ReleasedStream, vers.Series, nil)
		c.Assert(err, jc.ErrorIsNil)
		icfg.Jobs = []multiwatcher.MachineJob{multiwatcher.JobHostUnits}
	}
	err = icfg.SetTools(tools.List{
		&tools.Tools{
			Version: vers,
			URL:     "http://testing.invalid/tools.tar.gz",
		},
	})
	err = instancecfg.FinishInstanceConfig(icfg, modelConfig)
	c.Assert(err, jc.ErrorIsNil)
	cloudcfg, err := cloudinit.New(icfg.Series)
	c.Assert(err, jc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(icfg, cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	c.Assert(err, jc.ErrorIsNil)
	return cloudcfg
}

var allSeries = []string{"precise", "quantal", "raring", "saucy"}

func checkIff(checker gc.Checker, condition bool) gc.Checker {
	if condition {
		return checker
	}
	return gc.Not(checker)
}

var aptgetRegexp = "(.|\n)*" + regexp.QuoteMeta("apt-get --option=Dpkg::Options::=--force-confold --option=Dpkg::options::=--force-unsafe-io --assume-yes --quiet ")

func (s *configureSuite) TestAptSources(c *gc.C) {
	for _, series := range allSeries {
		vers := version.MustParseBinary("1.16.0-" + series + "-amd64")
		script, err := s.getCloudConfig(c, true, vers).RenderScript()
		c.Assert(err, jc.ErrorIsNil)

		// Only Precise requires the cloud-tools pocket.
		//
		// The only source we add that requires an explicitly
		// specified key is cloud-tools.
		needsCloudTools := series == "precise"
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools),
			"(.|\n)*apt-key add.*(.|\n)*",
		)
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools),
			"(.|\n)*add-apt-repository.*cloud-tools(.|\n)*",
		)
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools),
			"(.|\n)*Pin: release n=precise-updates/cloud-tools\nPin-Priority: 400(.|\n)*",
		)
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools),
			"(.|\n)*install -D -m 644 /dev/null '/etc/apt/preferences.d/50-cloud-tools'(.|\n)*",
		)

		// Only install python-software-properties (add-apt-repository)
		// if we need to.
		c.Assert(
			script,
			checkIff(gc.Matches, needsCloudTools),
			aptgetRegexp+"install.*python-software-properties(.|\n)*",
		)
	}
}

func assertScriptMatches(c *gc.C, cfg cloudinit.CloudConfig, pattern string, match bool) {
	script, err := cfg.RenderScript()
	c.Assert(err, jc.ErrorIsNil)
	checker := gc.Matches
	if !match {
		checker = gc.Not(checker)
	}
	c.Assert(script, checker, pattern)
}

func (s *configureSuite) TestAptUpdate(c *gc.C) {
	// apt-get update is run only if AptUpdate is set.
	aptGetUpdatePattern := aptgetRegexp + "update(.|\n)*"
	cfg, err := cloudinit.New("quantal")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cfg.SystemUpdate(), jc.IsFalse)
	c.Assert(cfg.PackageSources(), gc.HasLen, 0)
	assertScriptMatches(c, cfg, aptGetUpdatePattern, false)

	cfg.SetSystemUpdate(true)
	assertScriptMatches(c, cfg, aptGetUpdatePattern, true)

	// If we add sources, but disable updates, display an error.
	cfg.SetSystemUpdate(false)
	source := packaging.PackageSource{
		Name: "source",
		URL:  "source",
		Key:  "key",
	}
	cfg.AddPackageSource(source)
	_, err = cfg.RenderScript()
	c.Check(err, gc.ErrorMatches, "update sources were specified, but OS updates have been disabled.")
}

func (s *configureSuite) TestAptUpgrade(c *gc.C) {
	// apt-get upgrade is only run if AptUpgrade is set.
	aptGetUpgradePattern := aptgetRegexp + "upgrade(.|\n)*"
	cfg, err := cloudinit.New("quantal")
	c.Assert(err, jc.ErrorIsNil)
	cfg.SetSystemUpdate(true)
	source := packaging.PackageSource{
		Name: "source",
		URL:  "source",
		Key:  "key",
	}
	cfg.AddPackageSource(source)
	assertScriptMatches(c, cfg, aptGetUpgradePattern, false)
	cfg.SetSystemUpgrade(true)
	assertScriptMatches(c, cfg, aptGetUpgradePattern, true)
}

func (s *configureSuite) TestAptMirrorWrapper(c *gc.C) {
	expectedCommands := regexp.QuoteMeta(`
echo 'Changing apt mirror to http://woat.com' >&9
old_mirror=$(awk "/^deb .* $(lsb_release -sc) .*main.*\$/{print \$2;exit}" /etc/apt/sources.list)
new_mirror=http://woat.com
sed -i s,$old_mirror,$new_mirror, /etc/apt/sources.list
old_prefix=/var/lib/apt/lists/$(echo $old_mirror | sed 's,.*://,,' | sed 's,/$,,' | tr / _)
new_prefix=/var/lib/apt/lists/$(echo $new_mirror | sed 's,.*://,,' | sed 's,/$,,' | tr / _)
[ "$old_prefix" != "$new_prefix" ] &&
for old in ${old_prefix}_*; do
    new=$(echo $old | sed s,^$old_prefix,$new_prefix,)
    mv $old $new
done`)
	aptMirrorRegexp := "(.|\n)*" + expectedCommands + "(.|\n)*"
	cfg, err := cloudinit.New("quantal")
	c.Assert(err, jc.ErrorIsNil)
	cfg.SetPackageMirror("http://woat.com")
	assertScriptMatches(c, cfg, aptMirrorRegexp, true)
}
