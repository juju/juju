// Copyright 2012, 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudconfig_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/proxy"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/crypto/ssh"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	charmtesting "github.com/juju/juju/internal/charm/testing"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testcharms"
)

type cloudinitSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&cloudinitSuite{})

var (
	envConstraints       = constraints.MustParse("mem=2G")
	bootstrapConstraints = constraints.MustParse("mem=4G")

	allMachineJobs = []model.MachineJob{
		model.JobManageModel,
		model.JobHostUnits,
	}
	normalMachineJobs = []model.MachineJob{
		model.JobHostUnits,
	}

	jammy = corebase.MakeDefaultBase("ubuntu", "22.04")
	focal = corebase.MakeDefaultBase("ubuntu", "20.04")
)

func jujuLogDir(osName string) string {
	return path.Join(paths.LogDir(paths.OSType(osName)), "juju")
}

func jujuDataDir(osname string) string {
	return paths.DataDir(paths.OSType(osname))
}

func jujuTransientDataDir(osname string) string {
	return paths.TransientDataDir(paths.OSType(osname))
}

func cloudInitOutputLog(logDir string) string {
	return path.Join(logDir, "cloud-init-output.log")
}

func metricsSpoolDir(osname string) string {
	return paths.MetricsSpoolDir(paths.OSType(osname))
}

var stateServingInfo = controller.StateServingInfo{
	Cert:         string(serverCert),
	PrivateKey:   string(serverKey),
	CAPrivateKey: "ca-private-key",
	StatePort:    37017,
	APIPort:      17070,
}

// testcfg wraps InstanceConfig and provides helpers to modify it as
// needed for specific test cases before using it. Most methods return
// the method receiver (cfg) after (possibly) modifying it to allow
// chaining calls.
type testInstanceConfig instancecfg.InstanceConfig

// makeTestConfig returns a minimal instance config for a non state
// server machine (unless bootstrap is true) for the given series.
func makeTestConfig(base corebase.Base, bootstrap bool, vers semversion.Number, build int) *testInstanceConfig {
	const defaultMachineID = "99"

	cfg := new(testInstanceConfig)
	cfg.ControllerTag = testing.ControllerTag
	cfg.AuthorizedKeys = "sshkey1"
	cfg.AgentEnvironment = map[string]string{
		agent.ProviderType: "dummy",
	}
	cfg.MachineNonce = "FAKE_NONCE"
	cfg.Jobs = normalMachineJobs
	cfg.MetricsSpoolDir = metricsSpoolDir(base.OS)
	// APIInfo (sans Tag) must be initialized before calling setMachineID().
	cfg.APIInfo = &api.Info{
		Addrs:    []string{"state-addr.testing.invalid:54321"},
		Password: "bletch",
		CACert:   "CA CERT\n" + testing.CACert,
		ModelTag: testing.ModelTag,
	}
	cfg.setMachineID(defaultMachineID)
	cfg.setBase(base, vers, build)
	if bootstrap {
		return cfg.setController()
	} else {
		// Non-controller machines fetch their tools from
		// the controller.
		icfg := (*instancecfg.InstanceConfig)(cfg)
		toolsList := icfg.ToolsList()
		for i, tools := range toolsList {
			tools.URL = fmt.Sprintf(
				"https://%s/%s/tools/%s",
				cfg.APIInfo.Addrs[0],
				testing.ModelTag.Id(),
				tools.Version,
			)
			toolsList[i] = tools
		}
		if err := icfg.SetTools(toolsList); err != nil {
			panic(err)
		}
	}

	return cfg
}

// makeBootstrapConfig is a shortcut to call makeTestConfig(series, true).
func makeBootstrapConfig(base corebase.Base, build int) *testInstanceConfig {
	return makeTestConfig(base, true, semversion.MustParse("1.2.3"), build)
}

// makeNormalConfig is a shortcut to call makeTestConfig(series,
// false).
func makeNormalConfig(base corebase.Base, build int) *testInstanceConfig {
	return makeTestConfig(base, false, semversion.MustParse("1.2.3"), build)
}

// setMachineID updates MachineId, MachineAgentServiceName,
// MongoInfo.Tag, and APIInfo.Tag to match the given machine ID. If
// MongoInfo or APIInfo are nil, they're not changed.
func (cfg *testInstanceConfig) setMachineID(id string) *testInstanceConfig {
	cfg.MachineId = id
	cfg.MachineAgentServiceName = fmt.Sprintf("jujud-%s", names.NewMachineTag(id).String())
	if cfg.APIInfo != nil {
		cfg.APIInfo.Tag = names.NewMachineTag(id)
	}
	return cfg
}

func (cfg *testInstanceConfig) setControllerCharm(path string) *testInstanceConfig {
	cfg.Bootstrap.ControllerCharm = path
	return cfg
}

// maybeSetModelConfig sets the Config field to the given envConfig, if not
// nil, and the instance config is for a bootstrap machine.
func (cfg *testInstanceConfig) maybeSetModelConfig(envConfig *config.Config) *testInstanceConfig {
	if envConfig != nil && cfg.Bootstrap != nil {
		cfg.Bootstrap.ControllerModelConfig = envConfig
	}
	return cfg
}

// setEnableOSUpdateAndUpgrade sets EnableOSRefreshUpdate and EnableOSUpgrade
// fields to the given values.
func (cfg *testInstanceConfig) setEnableOSUpdateAndUpgrade(updateEnabled, upgradeEnabled bool) *testInstanceConfig {
	cfg.EnableOSRefreshUpdate = updateEnabled
	cfg.EnableOSUpgrade = upgradeEnabled
	return cfg
}

// setBase sets the series-specific fields (Tools, Release, DataDir,
// LogDir, and CloudInitOutputLog) to match the given series.
func (cfg *testInstanceConfig) setBase(base corebase.Base, vers semversion.Number, build int) *testInstanceConfig {
	ver := ""
	if build > 0 {
		ver = fmt.Sprintf("%s.%d-%s-amd64", vers.String(), build, base.OS)
	} else {
		ver = fmt.Sprintf("%s-%s-amd64", vers.String(), base.OS)
	}
	err := ((*instancecfg.InstanceConfig)(cfg)).SetTools(tools.List{
		newSimpleTools(ver),
	})
	if err != nil {
		panic(err)
	}
	cfg.Base = base
	cfg.DataDir = jujuDataDir(base.OS)
	cfg.TransientDataDir = jujuTransientDataDir(base.OS)
	cfg.LogDir = jujuLogDir(base.OS)
	cfg.CloudInitOutputLog = cloudInitOutputLog(base.OS)
	return cfg
}

// setController updates the config to be suitable for bootstrapping
// a controller instance.
func (cfg *testInstanceConfig) setController() *testInstanceConfig {
	cfg.setMachineID("0")
	cfg.ControllerConfig = controller.Config{}
	cfg.Bootstrap = &instancecfg.BootstrapConfig{
		StateInitializationParams: instancecfg.StateInitializationParams{
			BootstrapMachineInstanceId:  "i-bootstrap",
			BootstrapMachineConstraints: bootstrapConstraints,
			ModelConstraints:            envConstraints,
		},
		StateServingInfo: stateServingInfo,
		Timeout:          time.Minute * 10,
	}
	cfg.Jobs = allMachineJobs
	cfg.APIInfo.Tag = nil
	return cfg.setEnableOSUpdateAndUpgrade(true, false)
}

// mutate calls mutator passing cfg to it, and returns the (possibly)
// modified cfg.
func (cfg *testInstanceConfig) mutate(mutator func(*testInstanceConfig)) *testInstanceConfig {
	if mutator == nil {
		panic("mutator is nil!")
	}
	mutator(cfg)
	return cfg
}

// render returns the config as InstanceConfig.
func (cfg *testInstanceConfig) render() instancecfg.InstanceConfig {
	return instancecfg.InstanceConfig(*cfg)
}

type cloudinitTest struct {
	cfg           *testInstanceConfig
	setEnvConfig  bool
	expectScripts string
	// inexactMatch signifies whether we allow extra lines
	// in the actual scripts found. If it's true, the lines
	// mentioned in expectScripts must appear in that
	// order, but they can be arbitrarily interleaved with other
	// script lines.
	inexactMatch      bool
	upgradedToVersion string
}

func minimalModelConfig(c *tc.C) *config.Config {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig().Delete("authorized-keys"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, tc.NotNil)
	return cfg
}

// Each test gives a cloudinit config - we check the
// output to see if it looks correct.
var cloudinitTests = []cloudinitTest{
	// Test that cloudinit respects update/upgrade settings.
	{
		cfg:          makeBootstrapConfig(jammy, 0).setEnableOSUpdateAndUpgrade(false, false),
		inexactMatch: true,
		// We're just checking for apt-flags. We don't much care if
		// the script matches.
		expectScripts:     "",
		setEnvConfig:      true,
		upgradedToVersion: "1.2.3",
	},

	// Test that cloudinit respects update/upgrade settings.
	{
		cfg:          makeBootstrapConfig(jammy, 0).setEnableOSUpdateAndUpgrade(true, false),
		inexactMatch: true,
		// We're just checking for apt-flags. We don't much care if
		// the script matches.
		expectScripts:     "",
		setEnvConfig:      true,
		upgradedToVersion: "1.2.3",
	},

	// Test that cloudinit respects update/upgrade settings.
	{
		cfg:          makeBootstrapConfig(jammy, 0).setEnableOSUpdateAndUpgrade(false, true),
		inexactMatch: true,
		// We're just checking for apt-flags. We don't much care if
		// the script matches.
		expectScripts:     "",
		setEnvConfig:      true,
		upgradedToVersion: "1.2.3",
	},

	// Test that cloudinit respects update/upgrade settings.
	{
		cfg:          makeBootstrapConfig(jammy, 0).setEnableOSUpdateAndUpgrade(true, true),
		inexactMatch: true,
		// We're just checking for apt-flags. We don't much care if
		// the script matches.
		expectScripts:     "",
		setEnvConfig:      true,
		upgradedToVersion: "1.2.3",
	},

	// jammy controller
	{
		cfg:               makeBootstrapConfig(jammy, 0),
		inexactMatch:      true,
		setEnvConfig:      true,
		upgradedToVersion: "1.2.3",
		expectScripts: `
set -xe
install -D -m 644 /dev/null '/var/lib/juju/nonce.txt'
echo 'FAKE_NONCE' > '/var/lib/juju/nonce.txt'
test -n "\$JUJU_PROGRESS_FD" \|\| \(exec \{JUJU_PROGRESS_FD\}>&2\) 2>/dev/null && exec \{JUJU_PROGRESS_FD\}>&2 \|\| JUJU_PROGRESS_FD=2
if \[ ! -e /etc/profile.d/juju-proxy.sh \]; then.* >> /etc/profile.d/juju-proxy.sh\\nfi\\n
mkdir -p /var/lib/juju/locks
\(id ubuntu &> /dev/null\) && chown ubuntu:ubuntu /var/lib/juju/locks
mkdir -p /var/log/juju
chown syslog:adm /var/log/juju
bin='/var/lib/juju/tools/1\.2\.3-ubuntu-amd64'
mkdir -p \$bin
echo 'Fetching Juju agent version.*
.* curl .* --retry 10 -o \$bin/tools\.tar\.gz 'http://foo\.com/tools/released/juju1\.2\.3-ubuntu-amd64\.tgz'.*
sha256sum \$bin/tools\.tar\.gz > \$bin/juju1\.2\.3-ubuntu-amd64\.sha256
grep '1234' \$bin/juju1\.2\.3-ubuntu-amd64.sha256 \|\| \(echo "Tools checksum mismatch"; exit 1\)
tar zxf \$bin/tools.tar.gz -C \$bin
echo -n '{"version":"1\.2\.3-ubuntu-amd64","url":"http://foo\.com/tools/released/juju1\.2\.3-ubuntu-amd64\.tgz","sha256":"1234","size":10}' > \$bin/downloaded-tools\.txt
mkdir -p '/var/lib/juju/agents/machine-0'
cat > '/var/lib/juju/agents/machine-0/agent\.conf' << 'EOF'\\n.*\\nEOF
chmod 0600 '/var/lib/juju/agents/machine-0/agent\.conf'
install -D -m 600 /dev/null '/var/lib/juju/bootstrap-params'
echo '.*' > '/var/lib/juju/bootstrap-params'
echo 'Installing Juju machine agent'.*
/var/lib/juju/tools/1\.2\.3-ubuntu-amd64/jujud bootstrap-state --timeout 10m0s --data-dir '/var/lib/juju' --debug
/sbin/remove-juju-services
`,
	},

	// jammy controller with build in version
	{
		cfg:               makeBootstrapConfig(jammy, 123),
		inexactMatch:      true,
		setEnvConfig:      true,
		upgradedToVersion: "1.2.3.123",
		expectScripts: `
set -xe
install -D -m 644 /dev/null '/var/lib/juju/nonce.txt'
echo 'FAKE_NONCE' > '/var/lib/juju/nonce.txt'
test -n "\$JUJU_PROGRESS_FD" \|\| \(exec \{JUJU_PROGRESS_FD\}>&2\) 2>/dev/null && exec \{JUJU_PROGRESS_FD\}>&2 \|\| JUJU_PROGRESS_FD=2
if \[ ! -e /etc/profile.d/juju-proxy.sh \]; then.* >> /etc/profile.d/juju-proxy.sh\\nfi\\n
mkdir -p /var/lib/juju/locks
\(id ubuntu &> /dev/null\) && chown ubuntu:ubuntu /var/lib/juju/locks
mkdir -p /var/log/juju
chown syslog:adm /var/log/juju
bin='/var/lib/juju/tools/1\.2\.3\.123-ubuntu-amd64'
mkdir -p \$bin
echo 'Fetching Juju agent version.*
curl .* --retry 10 -o \$bin/tools\.tar\.gz 'http://foo\.com/tools/released/juju1\.2\.3\.123-ubuntu-amd64\.tgz'
sha256sum \$bin/tools\.tar\.gz > \$bin/juju1\.2\.3\.123-ubuntu-amd64\.sha256
grep '1234' \$bin/juju1\.2\.3\.123-ubuntu-amd64.sha256 \|\| \(echo "Tools checksum mismatch"; exit 1\)
tar zxf \$bin/tools.tar.gz -C \$bin
echo -n '{"version":"1\.2\.3\.123-ubuntu-amd64","url":"http://foo\.com/tools/released/juju1\.2\.3\.123-ubuntu-amd64\.tgz","sha256":"1234","size":10}' > \$bin/downloaded-tools\.txt
mkdir -p '/var/lib/juju/agents/machine-0'
cat > '/var/lib/juju/agents/machine-0/agent\.conf' << 'EOF'\\n.*\\nEOF
chmod 0600 '/var/lib/juju/agents/machine-0/agent\.conf'
install -D -m 600 /dev/null '/var/lib/juju/bootstrap-params'
echo '.*' > '/var/lib/juju/bootstrap-params'
echo 'Installing Juju machine agent'.*
/var/lib/juju/tools/1\.2\.3\.123-ubuntu-amd64/jujud bootstrap-state --timeout 10m0s --data-dir '/var/lib/juju' --debug
`,
	},

	// focal non controller.
	{
		cfg:               makeNormalConfig(focal, 0),
		inexactMatch:      true,
		upgradedToVersion: "1.2.3",
		expectScripts: `
set -xe
install -D -m 644 /dev/null '/var/lib/juju/nonce.txt'
echo 'FAKE_NONCE' > '/var/lib/juju/nonce.txt'
test -n "\$JUJU_PROGRESS_FD" \|\| \(exec \{JUJU_PROGRESS_FD\}>&2\) 2>/dev/null && exec \{JUJU_PROGRESS_FD\}>&2 \|\| JUJU_PROGRESS_FD=2
if \[ ! -e /etc/profile.d/juju-proxy.sh \]; then.* >> /etc/profile.d/juju-proxy.sh\\nfi\\n
mkdir -p /var/lib/juju/locks
\(id ubuntu &> /dev/null\) && chown ubuntu:ubuntu /var/lib/juju/locks
mkdir -p /var/log/juju
chown syslog:adm /var/log/juju
bin='/var/lib/juju/tools/1\.2\.3-ubuntu-amd64'
mkdir -p \$bin
echo 'Fetching Juju agent version.*
.* curl -sSf --connect-timeout 20 --noproxy "\*" --insecure -o \$bin/tools\.tar\.gz 'https://state-addr\.testing\.invalid:54321/deadbeef-0bad-400d-8000-4b1d0d06f00d/tools/1\.2\.3-ubuntu-amd64'.*
sha256sum \$bin/tools\.tar\.gz > \$bin/juju1\.2\.3-ubuntu-amd64\.sha256
grep '1234' \$bin/juju1\.2\.3-ubuntu-amd64.sha256 \|\| \(echo "Tools checksum mismatch"; exit 1\)
tar zxf \$bin/tools.tar.gz -C \$bin
echo -n '{"version":"1\.2\.3-ubuntu-amd64","url":"https://state-addr\.testing\.invalid:54321/deadbeef-0bad-400d-8000-4b1d0d06f00d/tools/1\.2\.3-ubuntu-amd64","sha256":"1234","size":10}' > \$bin/downloaded-tools\.txt
mkdir -p '/var/lib/juju/agents/machine-99'
cat > '/var/lib/juju/agents/machine-99/agent\.conf' << 'EOF'\\n.*\\nEOF
chmod 0600 '/var/lib/juju/agents/machine-99/agent\.conf'
`,
	},

	// check that it works ok with compound machine ids.
	{
		cfg: makeNormalConfig(jammy, 0).mutate(func(cfg *testInstanceConfig) {
			cfg.MachineContainerType = "lxd"
		}).setMachineID("2/lxd/1"),
		inexactMatch:      true,
		upgradedToVersion: "1.2.3",
		expectScripts: `
mkdir -p '/var/lib/juju/agents/machine-2-lxd-1'
cat > '/var/lib/juju/agents/machine-2-lxd-1/agent\.conf' << 'EOF'\\n.*\\nEOF
chmod 0600 '/var/lib/juju/agents/machine-2-lxd-1/agent\.conf'
ln -s 1\.2\.3-ubuntu-amd64 '/var/lib/juju/tools/machine-2-lxd-1'
`,
	},

	// hostname verification disabled.
	{
		cfg: makeNormalConfig(jammy, 0).mutate(func(cfg *testInstanceConfig) {
			cfg.DisableSSLHostnameVerification = true
		}),
		inexactMatch:      true,
		upgradedToVersion: "1.2.3",
		expectScripts: `
curl .* --noproxy "\*" --insecure -o \$bin/tools\.tar\.gz 'https://state-addr\.testing\.invalid:54321/deadbeef-0bad-400d-8000-4b1d0d06f00d/tools/1\.2\.3-ubuntu-amd64'
`,
	},

	// empty bootstrap constraints.
	{
		cfg: makeBootstrapConfig(jammy, 0).mutate(func(cfg *testInstanceConfig) {
			cfg.Bootstrap.BootstrapMachineConstraints = constraints.Value{}
		}),
		setEnvConfig:      true,
		inexactMatch:      true,
		upgradedToVersion: "1.2.3",
		expectScripts: `
echo '.*bootstrap-machine-constraints: {}.*' > '/var/lib/juju/bootstrap-params'
`,
	},

	// empty environ constraints.
	{
		cfg: makeBootstrapConfig(jammy, 0).mutate(func(cfg *testInstanceConfig) {
			cfg.Bootstrap.ModelConstraints = constraints.Value{}
		}),
		setEnvConfig:      true,
		inexactMatch:      true,
		upgradedToVersion: "1.2.3",
		expectScripts: `
echo '.*model-constraints: {}.*' > '/var/lib/juju/bootstrap-params'
`,
	},

	// custom image metadata (at bootstrap).
	{
		cfg: makeBootstrapConfig(jammy, 0).mutate(func(cfg *testInstanceConfig) {
			cfg.Bootstrap.CustomImageMetadata = []*imagemetadata.ImageMetadata{{
				Id:         "image-id",
				Storage:    "ebs",
				VirtType:   "pv",
				Arch:       "amd64",
				Version:    "22.04",
				RegionName: "us-east1",
			}}
		}),
		setEnvConfig:      true,
		inexactMatch:      true,
		upgradedToVersion: "1.2.3",
		expectScripts: `
echo '.*custom-image-metadata:.*us-east1.*.*' > '/var/lib/juju/bootstrap-params'
`,
	},

	// custom image metadata signing key.
	{
		cfg: makeBootstrapConfig(jammy, 0).mutate(func(cfg *testInstanceConfig) {
			cfg.PublicImageSigningKey = "publickey"
		}),
		setEnvConfig:      true,
		inexactMatch:      true,
		upgradedToVersion: "1.2.3",
		expectScripts: `
install -D -m 644 /dev/null '.*publicsimplestreamskey'
echo 'publickey' > '.*publicsimplestreamskey'
`,
	},
}

func newSimpleTools(vers string) *tools.Tools {
	return &tools.Tools{
		URL:     "http://foo.com/tools/released/juju" + vers + ".tgz",
		Version: semversion.MustParseBinary(vers),
		Size:    10,
		SHA256:  "1234",
	}
}

func getAgentConfig(c *tc.C, tag string, scripts []string) (cfg string) {
	c.Assert(scripts, tc.Not(tc.HasLen), 0)
	re := regexp.MustCompile(`cat > .*agents/` + regexp.QuoteMeta(tag) + `/agent\.conf' << 'EOF'\n((\n|.)+)\nEOF`)
	found := false
	for _, s := range scripts {
		m := re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		cfg = m[1]
		found = true
	}
	c.Assert(found, jc.IsTrue)
	return cfg
}

// check that any --model-config $base64 is valid and matches t.cfg.Config
func checkEnvConfig(c *tc.C, cfg *config.Config, scripts []string) {
	args := getStateInitializationParams(c, scripts)
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, args.ControllerModelConfig.AllAttrs())
}

func getStateInitializationParams(c *tc.C, scripts []string) instancecfg.StateInitializationParams {
	var args instancecfg.StateInitializationParams
	c.Assert(scripts, tc.Not(tc.HasLen), 0)
	re := regexp.MustCompile(`echo '(?s:(.+))' > '/var/lib/juju/bootstrap-params'`)
	for _, s := range scripts {
		m := re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		str := strings.Replace(m[1], "'\"'\"'", "'", -1)
		err := args.Unmarshal([]byte(str))
		c.Assert(err, jc.ErrorIsNil)
		return args
	}
	c.Fatal("could not find state initialization params")
	panic("unreachable")
}

// TestCloudInit checks that the output from the various tests
// in cloudinitTests is well formed.
func (s *cloudinitSuite) TestCloudInit(c *tc.C) {
	for i, test := range cloudinitTests {

		c.Logf("test %d", i)
		var envConfig *config.Config
		if test.setEnvConfig {
			envConfig = minimalModelConfig(c)
		}
		testConfig := test.cfg.maybeSetModelConfig(envConfig).render()
		ci, err := cloudinit.New(testConfig.Base.OS)
		c.Assert(err, jc.ErrorIsNil)
		udata, err := cloudconfig.NewUserdataConfig(&testConfig, ci)
		c.Assert(err, jc.ErrorIsNil)
		err = udata.Configure()

		c.Assert(err, jc.ErrorIsNil)
		c.Check(ci, tc.NotNil)
		// render the cloudinit config to bytes, and then
		// back to a map so we can introspect it without
		// worrying about internal details of the cloudinit
		// package.
		data, err := ci.RenderYAML()
		c.Assert(err, jc.ErrorIsNil)

		configKeyValues := make(map[interface{}]interface{})
		err = goyaml.Unmarshal(data, &configKeyValues)
		c.Assert(err, jc.ErrorIsNil)

		if testConfig.EnableOSRefreshUpdate {
			c.Check(configKeyValues["package_update"], jc.IsTrue)
		} else {
			c.Check(configKeyValues["package_update"], jc.IsFalse)
		}

		if testConfig.EnableOSUpgrade {
			c.Check(configKeyValues["package_upgrade"], jc.IsTrue)
		} else {
			c.Check(configKeyValues["package_upgrade"], jc.IsFalse)
		}

		scripts := getScripts(configKeyValues)
		assertScriptMatch(c, scripts, test.expectScripts, !test.inexactMatch)
		if testConfig.Bootstrap != nil {
			checkEnvConfig(c, testConfig.Bootstrap.ControllerModelConfig, scripts)
		}

		// curl should always be installed, since it's required by jujud.
		checkPackage(c, configKeyValues, "curl", true)

		tag := names.NewMachineTag(testConfig.MachineId).String()
		acfg := getAgentConfig(c, tag, scripts)
		c.Assert(acfg, jc.Contains, "AGENT_SERVICE_NAME: jujud-"+tag)
		c.Assert(acfg, jc.Contains, fmt.Sprintf("upgradedToVersion: %s\n", test.upgradedToVersion))
	}
}

func checkCloudInitWithContent(c *tc.C, cfg *testInstanceConfig, expectedScripts string, expectedError string) {
	envConfig := minimalModelConfig(c)
	testConfig := cfg.maybeSetModelConfig(envConfig).render()
	ci, err := cloudinit.New(testConfig.Base.OS)
	c.Assert(err, jc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(&testConfig, ci)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	if expectedError != "" {
		c.Assert(err, tc.ErrorMatches, expectedError)
		return
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ci, tc.NotNil)
	data, err := ci.RenderYAML()
	c.Assert(err, jc.ErrorIsNil)

	configKeyValues := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, &configKeyValues)
	c.Assert(err, jc.ErrorIsNil)

	scripts := getScripts(configKeyValues)
	assertScriptMatch(c, scripts, expectedScripts, false)
}

func (*cloudinitSuite) TestCloudInitWithLocalControllerCharmDir(c *tc.C) {
	tmpDir := c.MkDir()
	controllerCharmPath := filepath.Join(tmpDir, "controller.charm")

	ch := testcharms.Repo.CharmDir("juju-controller")
	buf := bytes.NewBuffer(nil)
	err := ch.ArchiveTo(buf)
	c.Assert(err, jc.ErrorIsNil)
	content := buf.Bytes()

	err = ch.ArchiveToPath(controllerCharmPath)
	c.Assert(err, jc.ErrorIsNil)

	cfg := makeBootstrapConfig(jammy, 0).setControllerCharm(controllerCharmPath)
	base64Content := base64.StdEncoding.EncodeToString(content)
	expectedScripts := regexp.QuoteMeta(fmt.Sprintf(`chmod 0600 '/var/lib/juju/agents/machine-0/agent.conf'
install -D -m 644 /dev/null '/var/lib/juju/charms/controller.charm'
echo -n %s | base64 -d > '/var/lib/juju/charms/controller.charm'
`, base64Content))
	checkCloudInitWithContent(c, cfg, expectedScripts, "")
}

func (*cloudinitSuite) TestCloudInitWithLocalControllerCharmArchive(c *tc.C) {
	ch := testcharms.Repo.CharmDir("juju-controller")
	dir, err := charmtesting.ReadCharmDir(ch.Path)
	c.Assert(err, jc.ErrorIsNil)

	controllerCharmPath := filepath.Join(c.MkDir(), "controller.charm")
	f, err := os.Create(controllerCharmPath)
	c.Assert(err, jc.ErrorIsNil)
	err = dir.ArchiveTo(f)
	_ = f.Close()
	c.Assert(err, jc.ErrorIsNil)

	content, err := os.ReadFile(controllerCharmPath)
	c.Assert(err, jc.ErrorIsNil)

	cfg := makeBootstrapConfig(jammy, 0).setControllerCharm(controllerCharmPath)
	base64Content := base64.StdEncoding.EncodeToString(content)
	expectedScripts := regexp.QuoteMeta(fmt.Sprintf(`chmod 0600 '/var/lib/juju/agents/machine-0/agent.conf'
install -D -m 644 /dev/null '/var/lib/juju/charms/controller.charm'
echo -n %s | base64 -d > '/var/lib/juju/charms/controller.charm'
`, base64Content))
	checkCloudInitWithContent(c, cfg, expectedScripts, "")
}

func (*cloudinitSuite) TestCloudInitConfigure(c *tc.C) {
	for i, test := range cloudinitTests {
		testConfig := test.cfg.maybeSetModelConfig(minimalModelConfig(c)).render()
		c.Logf("test %d (Configure)", i)
		cloudcfg, err := cloudinit.New(testConfig.Base.OS)
		c.Assert(err, jc.ErrorIsNil)
		udata, err := cloudconfig.NewUserdataConfig(&testConfig, cloudcfg)
		c.Assert(err, jc.ErrorIsNil)
		err = udata.Configure()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *cloudinitSuite) TestCloudInitConfigCloudInitUserData(c *tc.C) {
	environConfig := minimalModelConfig(c)
	environConfig, err := environConfig.Apply(map[string]interface{}{
		config.CloudInitUserDataKey: validCloudInitUserData,
	})
	c.Assert(err, jc.ErrorIsNil)
	instanceCfg := s.createInstanceConfig(c, environConfig)
	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(instanceCfg, cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	c.Assert(err, jc.ErrorIsNil)

	// Verify the settings against cloudinit-userdata
	cfgPackages := cloudcfg.Packages()
	expectedPackages := []string{
		`tmux`, // last juju specified package
		`python-keystoneclient`,
		`python-glanceclient`,
	}
	c.Assert(len(cfgPackages), jc.GreaterThan, 2)
	c.Assert(cfgPackages[len(cfgPackages)-3:], tc.DeepEquals, expectedPackages)

	cmds := cloudcfg.RunCmds()
	beginning := []string{
		`mkdir /tmp/preruncmd`,
		`mkdir /tmp/preruncmd2`,
		`set -xe`, // first line of juju specified cmds
	}
	ending := []string{
		`mkdir /tmp/postruncmd`,
		`mkdir "/tmp/postruncmd 2"`,
	}
	c.Assert(len(cmds), jc.GreaterThan, 6)
	c.Assert(cmds[:3], tc.DeepEquals, beginning)
	c.Assert(cmds[len(cmds)-2:], tc.DeepEquals, ending)

	c.Assert(cloudcfg.SystemUpgrade(), tc.Equals, false)

	// Render to check for the "unexpected" cloudinit text.
	// cloudconfig doesn't have public access to all attrs.
	data, err := cloudcfg.RenderYAML()
	c.Assert(err, jc.ErrorIsNil)
	ciContent := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, &ciContent)
	c.Assert(err, jc.ErrorIsNil)
	testCmd, ok := ciContent["test-key"].([]interface{})
	c.Assert(ok, jc.IsTrue)
	c.Check(testCmd, tc.DeepEquals, []interface{}{"test line one"})
}

var validCloudInitUserData = `
packages:
  - 'python-keystoneclient'
  - 'python-glanceclient'
preruncmd:
  - mkdir /tmp/preruncmd
  - ["mkdir", "/tmp/preruncmd2"]
postruncmd:
  - mkdir /tmp/postruncmd
  - ["mkdir", "/tmp/postruncmd 2"]
package_upgrade: false
test-key:
  - test line one
`[1:]

func (*cloudinitSuite) bootstrapConfigScripts(c *tc.C) []string {
	loggo.GetLogger("").SetLogLevel(loggo.INFO)
	envConfig := minimalModelConfig(c)
	instConfig := makeBootstrapConfig(jammy, 0).maybeSetModelConfig(envConfig)
	rendered := instConfig.render()
	cloudcfg, err := cloudinit.New(rendered.Base.OS)
	c.Assert(err, jc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(&rendered, cloudcfg)

	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	c.Assert(err, jc.ErrorIsNil)
	data, err := cloudcfg.RenderYAML()
	c.Assert(err, jc.ErrorIsNil)
	configKeyValues := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, &configKeyValues)
	c.Assert(err, jc.ErrorIsNil)

	scripts := getScripts(configKeyValues)
	for i, script := range scripts {
		if strings.Contains(script, "bootstrap") {
			c.Logf("scripts[%d]: %q", i, script)
		}
	}
	return scripts
}

func (s *cloudinitSuite) TestCloudInitPreruncmdError(c *tc.C) {
	environConfig := minimalModelConfig(c)
	environConfig, err := environConfig.Apply(map[string]interface{}{
		config.CloudInitUserDataKey: `
preruncmd:
  - 42
`,
	})
	c.Assert(err, jc.ErrorIsNil)
	instanceCfg := s.createInstanceConfig(c, environConfig)
	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(instanceCfg, cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	c.Assert(err, tc.ErrorMatches, `invalid preruncmd: .* got int`)
}

func (s *cloudinitSuite) TestCloudInitPostruncmdError(c *tc.C) {
	environConfig := minimalModelConfig(c)
	environConfig, err := environConfig.Apply(map[string]interface{}{
		config.CloudInitUserDataKey: `
postruncmd:
  - ["foo", 3.14]
`,
	})
	c.Assert(err, jc.ErrorIsNil)
	instanceCfg := s.createInstanceConfig(c, environConfig)
	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(instanceCfg, cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	c.Assert(err, tc.ErrorMatches, `invalid postruncmd: .* got list containing float64`)
}

func (s *cloudinitSuite) TestCloudInitConfigureBootstrapLogging(c *tc.C) {
	scripts := s.bootstrapConfigScripts(c)
	expected := "jujud bootstrap-state .* --show-log"
	assertScriptMatch(c, scripts, expected, false)
}

func (s *cloudinitSuite) TestCloudInitConfigureBootstrapFeatureFlags(c *tc.C) {
	s.SetFeatureFlags("special", "foo")
	scripts := s.bootstrapConfigScripts(c)
	expected := "JUJU_DEV_FEATURE_FLAGS=foo,special .*/jujud bootstrap-state .*"
	assertScriptMatch(c, scripts, expected, false)
}

func (*cloudinitSuite) TestCloudInitConfigureUsesGivenConfig(c *tc.C) {
	// Create a simple cloudinit config with a 'runcmd' statement.
	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	script := "test script"
	cloudcfg.AddRunCmd(script)
	envConfig := minimalModelConfig(c)
	testConfig := cloudinitTests[0].cfg.maybeSetModelConfig(envConfig).render()
	udata, err := cloudconfig.NewUserdataConfig(&testConfig, cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	c.Assert(err, jc.ErrorIsNil)
	data, err := cloudcfg.RenderYAML()
	c.Assert(err, jc.ErrorIsNil)

	ciContent := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, &ciContent)
	c.Assert(err, jc.ErrorIsNil)
	// The 'runcmd' statement is at the beginning of the list
	// of 'runcmd' statements.
	runCmd := ciContent["runcmd"].([]interface{})
	c.Check(runCmd[0], tc.Equals, script)
}

func getScripts(configKeyValue map[interface{}]interface{}) []string {
	var scripts []string
	if bootcmds, ok := configKeyValue["bootcmd"]; ok {
		for _, s := range bootcmds.([]interface{}) {
			scripts = append(scripts, s.(string))
		}
	}
	for _, s := range configKeyValue["runcmd"].([]interface{}) {
		scripts = append(scripts, s.(string))
	}
	return scripts
}

type line struct {
	index int
	line  string
}

func assertScriptMatch(c *tc.C, got []string, expect string, exact bool) {

	// Convert string slice into line struct slice
	assembleLines := func(lines []string, lineProcessor func(string) string) []line {
		var assembledLines []line
		for lineIdx, currLine := range lines {
			if nil != lineProcessor {
				currLine = lineProcessor(currLine)
			}
			assembledLines = append(assembledLines, line{
				index: lineIdx,
				line:  currLine,
			})
		}
		return assembledLines
	}

	pats := assembleLines(strings.Split(strings.Trim(expect, "\n"), "\n"), nil)
	scripts := assembleLines(got, func(line string) string {
		return strings.Replace(line, "\n", "\\n", -1) // make .* work
	})

	// Pop patterns and scripts off the head as we find pairs
	for {
		switch {
		case len(pats) == 0 && len(scripts) == 0:
			return
		case len(pats) == 0:
			if exact {
				c.Fatalf("too many scripts found (got %q at line %d)", scripts[0].line, scripts[0].index)
			}
			return
		case len(scripts) == 0:
			if exact {
				c.Fatalf("too few scripts found (expected %q at line %d)", pats[0].line, pats[0].index)
			}
			c.Fatalf("could not find match for %q\ngot:\n%s", pats[0].line, strings.Join(got, "\n"))
		default:
			ok, err := regexp.MatchString(pats[0].line, scripts[0].line)
			c.Assert(err, jc.ErrorIsNil, tc.Commentf("invalid regexp: %q", pats[0].line))
			if ok {
				pats = pats[1:]
				scripts = scripts[1:]
			} else if exact {
				c.Assert(scripts[0].line, tc.Matches, pats[0].line, tc.Commentf("line %d;\nexpected %q;\ngot %q;\npaths: %#v", scripts[0].index, pats[0].line, scripts[0].line, pats))
			} else {
				scripts = scripts[1:]
			}
		}
	}
}

// checkPackage checks that the cloudinit will or won't install the given
// package, depending on the value of match.
func checkPackage(c *tc.C, x map[interface{}]interface{}, pkg string, match bool) {
	pkgs0 := x["packages"]
	if pkgs0 == nil {
		if match {
			c.Errorf("cloudinit has no entry for packages")
		}
		return
	}

	pkgs := pkgs0.([]interface{})

	found := false
	for _, p0 := range pkgs {
		p := p0.(string)
		// p might be a space separate list of packages eg 'foo bar qed' so split them up
		manyPkgs := set.NewStrings(strings.Split(p, " ")...)
		hasPkg := manyPkgs.Contains(pkg)
		if p == pkg || hasPkg {
			found = true
			break
		}
	}
	switch {
	case match && !found:
		c.Errorf("package %q not found in %v", pkg, pkgs)
	case !match && found:
		c.Errorf("%q found but not expected in %v", pkg, pkgs)
	}
}

// When mutate is called on a known-good InstanceConfig,
// there should be an error complaining about the missing
// field named by the adjacent err.
var verifyTests = []struct {
	err    string
	mutate func(*instancecfg.InstanceConfig)
}{
	{"invalid machine id", func(cfg *instancecfg.InstanceConfig) {
		cfg.MachineId = "-1"
	}},
	{"invalid bootstrap configuration: missing model configuration", func(cfg *instancecfg.InstanceConfig) {
		cfg.Bootstrap.ControllerModelConfig = nil
	}},
	{"missing API info", func(cfg *instancecfg.InstanceConfig) {
		cfg.APIInfo = nil
	}},
	{"missing model tag", func(cfg *instancecfg.InstanceConfig) {
		cfg.APIInfo = &api.Info{
			Addrs:  []string{"foo:35"},
			Tag:    names.NewMachineTag("99"),
			CACert: testing.CACert,
		}
	}},
	{"missing API hosts", func(cfg *instancecfg.InstanceConfig) {
		cfg.Bootstrap = nil
		cfg.APIInfo = &api.Info{
			Tag:      names.NewMachineTag("99"),
			CACert:   testing.CACert,
			ModelTag: testing.ModelTag,
		}
	}},
	{"invalid bootstrap configuration: missing controller certificate", func(cfg *instancecfg.InstanceConfig) {
		cfg.Bootstrap.StateServingInfo.Cert = ""
	}},
	{"invalid bootstrap configuration: missing controller private key", func(cfg *instancecfg.InstanceConfig) {
		cfg.Bootstrap.StateServingInfo.PrivateKey = ""
	}},
	{"invalid bootstrap configuration: missing ca cert private key", func(cfg *instancecfg.InstanceConfig) {
		cfg.Bootstrap.StateServingInfo.CAPrivateKey = ""
	}},
	{"invalid bootstrap configuration: missing state port", func(cfg *instancecfg.InstanceConfig) {
		cfg.Bootstrap.StateServingInfo.StatePort = 0
	}},
	{"invalid bootstrap configuration: missing API port", func(cfg *instancecfg.InstanceConfig) {
		cfg.Bootstrap.StateServingInfo.APIPort = 0
	}},
	{"missing var directory", func(cfg *instancecfg.InstanceConfig) {
		cfg.DataDir = ""
	}},
	{"missing log directory", func(cfg *instancecfg.InstanceConfig) {
		cfg.LogDir = ""
	}},
	{"missing cloud-init output log path", func(cfg *instancecfg.InstanceConfig) {
		cfg.CloudInitOutputLog = ""
	}},
	{"invalid bootstrap configuration: entity tag must be nil when bootstrapping", func(cfg *instancecfg.InstanceConfig) {
		cfg.APIInfo.Tag = names.NewMachineTag("0")
	}},
	{"missing machine nonce", func(cfg *instancecfg.InstanceConfig) {
		cfg.MachineNonce = ""
	}},
	{"missing machine agent service name", func(cfg *instancecfg.InstanceConfig) {
		cfg.MachineAgentServiceName = ""
	}},
	{"invalid bootstrap configuration: missing bootstrap machine instance ID", func(cfg *instancecfg.InstanceConfig) {
		cfg.Bootstrap.BootstrapMachineInstanceId = ""
	}},
}

// TestCloudInitVerify checks that required fields are appropriately
// checked for by NewCloudInit.
func (*cloudinitSuite) TestCloudInitVerify(c *tc.C) {
	toolsList := tools.List{
		newSimpleTools("9.9.9-ubuntu-arble"),
	}

	makeCfgWithoutTools := func() instancecfg.InstanceConfig {
		return instancecfg.InstanceConfig{
			Bootstrap: &instancecfg.BootstrapConfig{
				StateInitializationParams: instancecfg.StateInitializationParams{
					BootstrapMachineInstanceId: "i-bootstrap",
					ControllerModelConfig:      minimalModelConfig(c),
				},
				StateServingInfo: stateServingInfo,
			},
			ControllerConfig: controller.Config{},
			ControllerTag:    testing.ControllerTag,
			MachineId:        "99",
			AuthorizedKeys:   "sshkey1",
			Base:             jammy,
			AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
			APIInfo: &api.Info{
				Addrs:    []string{"host:9999"},
				CACert:   testing.CACert,
				ModelTag: testing.ModelTag,
				Password: "password",
			},
			DataDir:                 jujuDataDir("ubuntu"),
			LogDir:                  jujuLogDir("ubuntu"),
			MetricsSpoolDir:         metricsSpoolDir("ubuntu"),
			Jobs:                    normalMachineJobs,
			CloudInitOutputLog:      cloudInitOutputLog("jammy"),
			MachineNonce:            "FAKE_NONCE",
			MachineAgentServiceName: "jujud-machine-99",
		}
	}

	// check that the base configuration does not give an error
	ci, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)

	// check that missing tools causes an error.
	cfg := makeCfgWithoutTools()
	udata, err := cloudconfig.NewUserdataConfig(&cfg, ci)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	c.Assert(err, tc.ErrorMatches, "invalid machine configuration: missing agent binaries")

	for i, test := range verifyTests {
		c.Logf("test %d. %s", i, test.err)
		cfg := makeCfgWithoutTools()
		err := cfg.SetTools(toolsList)
		c.Assert(err, jc.ErrorIsNil)

		// check that the base configuration does not give an error
		udata, err := cloudconfig.NewUserdataConfig(&cfg, ci)
		c.Assert(err, jc.ErrorIsNil)
		err = udata.Configure()
		c.Assert(err, jc.ErrorIsNil)

		test.mutate(&cfg)
		udata, err = cloudconfig.NewUserdataConfig(&cfg, ci)
		c.Assert(err, jc.ErrorIsNil)
		err = udata.Configure()
		c.Check(err, tc.ErrorMatches, "invalid machine configuration: "+test.err)
	}
}

func (*cloudinitSuite) createInstanceConfig(c *tc.C, environConfig *config.Config) *instancecfg.InstanceConfig {
	machineId := "42"
	machineNonce := "fake-nonce"
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(testing.ControllerTag, machineId, machineNonce,
		imagemetadata.ReleasedStream, jammy, apiInfo)
	c.Assert(err, jc.ErrorIsNil)
	instanceConfig.SetTools(tools.List{
		&tools.Tools{
			Version: semversion.MustParseBinary("2.3.4-ubuntu-amd64"),
			URL:     "http://tools.testing.invalid/2.3.4-ubuntu-amd64.tgz",
		},
	})
	err = instancecfg.FinishInstanceConfig(instanceConfig, environConfig)
	c.Assert(err, jc.ErrorIsNil)
	return instanceConfig
}

func (s *cloudinitSuite) TestAptProxyNotWrittenIfNotSet(c *tc.C) {
	environConfig := minimalModelConfig(c)
	instanceCfg := s.createInstanceConfig(c, environConfig)
	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(instanceCfg, cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	c.Assert(err, jc.ErrorIsNil)

	cmds := cloudcfg.BootCmds()
	c.Assert(cmds, tc.IsNil)
}

func (s *cloudinitSuite) TestAptProxyWritten(c *tc.C) {
	environConfig := minimalModelConfig(c)
	environConfig, err := environConfig.Apply(map[string]interface{}{
		"apt-http-proxy": "http://user@10.0.0.1",
	})
	c.Assert(err, jc.ErrorIsNil)
	instanceCfg := s.createInstanceConfig(c, environConfig)
	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(instanceCfg, cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	c.Assert(err, jc.ErrorIsNil)

	cmds := cloudcfg.BootCmds()
	expected := "echo 'Acquire::http::Proxy \"http://user@10.0.0.1\";' > /etc/apt/apt.conf.d/95-juju-proxy-settings"
	c.Assert(cmds, jc.DeepEquals, []string{expected})
}

func (s *cloudinitSuite) TestProxyWritten(c *tc.C) {
	environConfig := minimalModelConfig(c)
	environConfig, err := environConfig.Apply(map[string]interface{}{
		"http-proxy": "http://user@10.0.0.1",
		"no-proxy":   "localhost,10.0.3.1",
	})
	c.Assert(err, jc.ErrorIsNil)
	instanceCfg := s.createInstanceConfig(c, environConfig)
	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(instanceCfg, cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	c.Assert(err, jc.ErrorIsNil)

	cmds := cloudcfg.RunCmds()
	first := cloudconfig.JujuProxyProfileScript
	expected := []string{
		`export http_proxy=http://user@10.0.0.1`,
		`export HTTP_PROXY=http://user@10.0.0.1`,
		`export no_proxy=0.1.2.3,10.0.3.1,localhost`,
		`export NO_PROXY=0.1.2.3,10.0.3.1,localhost`,
		``,
		`(echo 'export http_proxy=http://user@10.0.0.1
export HTTP_PROXY=http://user@10.0.0.1
export no_proxy=0.1.2.3,10.0.3.1,localhost
export NO_PROXY=0.1.2.3,10.0.3.1,localhost
' > /etc/juju-proxy.conf && chmod 0644 /etc/juju-proxy.conf)`,
		`echo '# To allow juju to control the global systemd proxy settings,
# create symbolic links to this file from within /etc/systemd/system.conf.d/
# and /etc/systemd/users.conf.d/.
[Manager]
DefaultEnvironment="http_proxy=http://user@10.0.0.1" "HTTP_PROXY=http://user@10.0.0.1" "no_proxy=0.1.2.3,10.0.3.1,localhost" "NO_PROXY=0.1.2.3,10.0.3.1,localhost" 
' > /etc/juju-proxy-systemd.conf`,
	}
	found := false
	for i, cmd := range cmds {
		if cmd == first {
			c.Assert(cmds[i+1:i+8], jc.DeepEquals, expected, tc.Commentf("obtained (%s)", cmds[i+1:i+8]))
			found = true
			break
		}
	}
	c.Logf("\n%s\n", cmds)
	c.Assert(found, jc.IsTrue)
}

// Ensure the bootstrap curl which fetch tools respects the proxy settings
func (s *cloudinitSuite) TestProxyArgsAddedToCurlCommand(c *tc.C) {
	instcfg := makeBootstrapConfig(jammy, 0).maybeSetModelConfig(
		minimalModelConfig(c),
	).render()
	instcfg.JujuProxySettings = proxy.Settings{
		Http: "0.1.2.3",
	}

	// create the cloud configuration
	cldcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)

	// create the user data configuration setup
	udata, err := cloudconfig.NewUserdataConfig(&instcfg, cldcfg)
	c.Assert(err, jc.ErrorIsNil)

	// configure the user data
	err = udata.Configure()
	c.Assert(err, jc.ErrorIsNil)

	// check to see that the first boot curl command to download tools
	// respects the configured proxy settings.
	cmds := cldcfg.RunCmds()
	expectedCurlCommand := "curl -sSf --retry 10 --proxy 0.1.2.3 -o $bin/tools.tar.gz"
	assertCommandsContain(c, cmds, expectedCurlCommand)
}

// search a list of first boot commands to ensure it contains the specified curl
// command.
func assertCommandsContain(c *tc.C, runCmds []string, str string) {
	found := false
	for _, runCmd := range runCmds {
		if strings.Contains(runCmd, str) {
			found = true
		}
	}
	c.Logf("expecting to find %q in %#v", str, runCmds)
	c.Assert(found, jc.IsTrue)
}

func (s *cloudinitSuite) TestAptMirror(c *tc.C) {
	environConfig := minimalModelConfig(c)
	environConfig, err := environConfig.Apply(map[string]interface{}{
		"apt-mirror": "http://my.archive.ubuntu.com/ubuntu",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.testAptMirror(c, environConfig, "http://my.archive.ubuntu.com/ubuntu")
}

func (s *cloudinitSuite) TestAptMirrorNotSet(c *tc.C) {
	environConfig := minimalModelConfig(c)
	s.testAptMirror(c, environConfig, "")
}

func (s *cloudinitSuite) testAptMirror(c *tc.C, cfg *config.Config, expect string) {
	instanceCfg := s.createInstanceConfig(c, cfg)
	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(instanceCfg, cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	c.Assert(err, jc.ErrorIsNil)
	//mirror, ok := cloudcfg.AptMirror()
	mirror := cloudcfg.PackageMirror()
	c.Assert(mirror, tc.Equals, expect)
	//c.Assert(ok, gc.Equals, expect != "")
}

var serverCert = []byte(`
SERVER CERT
-----BEGIN CERTIFICATE-----
MIIBdzCCASOgAwIBAgIBADALBgkqhkiG9w0BAQUwHjENMAsGA1UEChMEanVqdTEN
MAsGA1UEAxMEcm9vdDAeFw0xMjExMDgxNjIyMzRaFw0xMzExMDgxNjI3MzRaMBwx
DDAKBgNVBAoTA2htbTEMMAoGA1UEAxMDYW55MFowCwYJKoZIhvcNAQEBA0sAMEgC
QQCACqz6JPwM7nbxAWub+APpnNB7myckWJ6nnsPKi9SipP1hyhfzkp8RGMJ5Uv7y
8CSTtJ8kg/ibka1VV8LvP9tnAgMBAAGjUjBQMA4GA1UdDwEB/wQEAwIAsDAdBgNV
HQ4EFgQU6G1ERaHCgfAv+yoDMFVpDbLOmIQwHwYDVR0jBBgwFoAUP/mfUdwOlHfk
fR+gLQjslxf64w0wCwYJKoZIhvcNAQEFA0EAbn0MaxWVgGYBomeLYfDdb8vCq/5/
G/2iCUQCXsVrBparMLFnor/iKOkJB5n3z3rtu70rFt+DpX6L8uBR3LB3+A==
-----END CERTIFICATE-----
`[1:])

var serverKey = []byte(`
SERVER KEY
-----BEGIN RSA PRIVATE KEY-----
MIIBPAIBAAJBAIAKrPok/AzudvEBa5v4A+mc0HubJyRYnqeew8qL1KKk/WHKF/OS
nxEYwnlS/vLwJJO0nySD+JuRrVVXwu8/22cCAwEAAQJBAJsk1F0wTRuaIhJ5xxqw
FIWPFep/n5jhrDOsIs6cSaRbfIBy3rAl956pf/MHKvf/IXh7KlG9p36IW49hjQHK
7HkCIQD2CqyV1ppNPFSoCI8mSwO8IZppU3i2V4MhpwnqHz3H0wIhAIU5XIlhLJW8
TNOaFMEia/TuYofdwJnYvi9t0v4UKBWdAiEA76AtvjEoTpi3in/ri0v78zp2/KXD
JzPMDvZ0fYS30ukCIA1stlJxpFiCXQuFn0nG+jH4Q52FTv8xxBhrbLOFvHRRAiEA
2Vc9NN09ty+HZgxpwqIA1fHVuYJY9GMPG1LnTnZ9INg=
-----END RSA PRIVATE KEY-----
`[1:])

func (*cloudinitSuite) TestToolsDownloadCommand(c *tc.C) {
	command := cloudconfig.ToolsDownloadCommand("download", []string{"a", "b", "c"})

	expected := `
n=1
while true; do

    echo "Attempt $n to download agent binaries from 'a'...\n"
    download 'a' && echo "Agent binaries downloaded successfully." && break

    echo "Attempt $n to download agent binaries from 'b'...\n"
    download 'b' && echo "Agent binaries downloaded successfully." && break

    echo "Attempt $n to download agent binaries from 'c'...\n"
    download 'c' && echo "Agent binaries downloaded successfully." && break

    echo "Download failed, retrying in 15s"
    sleep 15
    n=$((n+1))
done`
	c.Assert(command, tc.Equals, expected)
}

func expectedUbuntuUser(groups, keys []string) map[string]interface{} {
	user := map[string]interface{}{
		"name":        "ubuntu",
		"lock_passwd": true,
		"shell":       "/bin/bash",
		"sudo":        "ALL=(ALL) NOPASSWD:ALL",
	}
	if groups != nil {
		user["groups"] = groups
	}
	if keys != nil {
		user["ssh_authorized_keys"] = keys
	}
	return map[string]interface{}{
		"users": []map[string]interface{}{user},
	}
}

func (*cloudinitSuite) TestSetUbuntuUserJammy(c *tc.C) {
	ci, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	cloudconfig.SetUbuntuUser(ci, "akey")
	data, err := ci.RenderYAML()
	c.Assert(err, jc.ErrorIsNil)
	keys := []string{"akey"}
	expected := expectedUbuntuUser(cloudconfig.UbuntuGroups, keys)
	c.Assert(string(data), jc.YAMLEquals, expected)
}

func (*cloudinitSuite) TestCloudInitBootstrapInitialSSHKeys(c *tc.C) {
	instConfig := makeBootstrapConfig(jammy, 0).maybeSetModelConfig(
		minimalModelConfig(c),
	).render()
	instConfig.Bootstrap.InitialSSHHostKeys = instancecfg.SSHHostKeys{{
		Private:            "private",
		Public:             "public",
		PublicKeyAlgorithm: ssh.KeyAlgoRSA,
	}}
	cloudcfg, err := cloudinit.New(instConfig.Base.OS)
	c.Assert(err, jc.ErrorIsNil)

	udata, err := cloudconfig.NewUserdataConfig(&instConfig, cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	c.Assert(err, jc.ErrorIsNil)
	data, err := cloudcfg.RenderYAML()
	c.Assert(err, jc.ErrorIsNil)

	configKeyValues := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, &configKeyValues)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(configKeyValues["ssh_keys"], jc.DeepEquals, map[interface{}]interface{}{
		"rsa_private": "private",
		"rsa_public":  "public",
	})

	cmds := cloudcfg.BootCmds()
	c.Assert(cmds, jc.DeepEquals, []string{
		`echo 'Regenerating SSH host keys' >&$JUJU_PROGRESS_FD`,
		`rm /etc/ssh/ssh_host_*_key*`,
		`ssh-keygen -t rsa -N "" -f /etc/ssh/ssh_host_rsa_key`,
		`ssh-keygen -t ecdsa -N "" -f /etc/ssh/ssh_host_ecdsa_key`,
		`ssh-keygen -t ed25519 -N "" -f /etc/ssh/ssh_host_ed25519_key || true`,
		`service ssh reload`,
	})
}
