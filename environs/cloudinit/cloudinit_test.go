// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"encoding/base64"
	"path"
	"regexp"
	"strings"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	coreCloudinit "github.com/juju/juju/cloudinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/juju/paths"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// Use local suite since this file lives in the ec2 package
// for testing internals.
type cloudinitSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&cloudinitSuite{})

var envConstraints = constraints.MustParse("mem=2G")

var allMachineJobs = []params.MachineJob{
	params.JobManageEnviron,
	params.JobHostUnits,
	params.JobManageNetworking,
}
var normalMachineJobs = []params.MachineJob{
	params.JobHostUnits,
}

type cloudinitTest struct {
	cfg           cloudinit.MachineConfig
	setEnvConfig  bool
	expectScripts string
	// inexactMatch signifies whether we allow extra lines
	// in the actual scripts found. If it's true, the lines
	// mentioned in expectScripts must appear in that
	// order, but they can be arbitrarily interleaved with other
	// script lines.
	inexactMatch bool
}

func minimalMachineConfig(tweakers ...func(cloudinit.MachineConfig)) cloudinit.MachineConfig {

	baseConfig := cloudinit.MachineConfig{
		MachineId:        "0",
		AuthorizedKeys:   "sshkey1",
		AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
		// raring provides mongo in the archive
		Tools:            newSimpleTools("1.2.3-raring-amd64"),
		Series:           "raring",
		Bootstrap:        true,
		StateServingInfo: stateServingInfo,
		MachineNonce:     "FAKE_NONCE",
		MongoInfo: &mongo.MongoInfo{
			Password: "arble",
			Info: mongo.Info{
				CACert: "CA CERT\n" + testing.CACert,
			},
		},
		APIInfo: &api.Info{
			Password: "bletch",
			CACert:   "CA CERT\n" + testing.CACert,
		},
		Constraints:             envConstraints,
		DataDir:                 environs.DataDir,
		LogDir:                  agent.DefaultLogDir,
		Jobs:                    allMachineJobs,
		CloudInitOutputLog:      cloudInitOutputLog,
		InstanceId:              "i-bootstrap",
		MachineAgentServiceName: "jujud-machine-0",
		EnableOSRefreshUpdate:   false,
		EnableOSUpgrade:         false,
	}

	for _, tweaker := range tweakers {
		tweaker(baseConfig)
	}

	return baseConfig
}

func minimalConfig(c *gc.C) *config.Config {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig())
	c.Assert(err, gc.IsNil)
	c.Assert(cfg, gc.NotNil)
	return cfg
}

func must(s string, err error) string {
	if err != nil {
		panic(err)
	}
	return s
}

var stateServingInfo = &params.StateServingInfo{
	Cert:       string(serverCert),
	PrivateKey: string(serverKey),
	StatePort:  37017,
	APIPort:    17070,
}

var logDir = must(paths.LogDir("precise"))
var cloudInitOutputLog = path.Join(logDir, "cloud-init-output.log")

// Each test gives a cloudinit config - we check the
// output to see if it looks correct.
var cloudinitTests = []cloudinitTest{
	// Test that cloudinit respects update/upgrade settings.
	{
		cfg: minimalMachineConfig(func(mc cloudinit.MachineConfig) {
			mc.EnableOSRefreshUpdate = false
			mc.EnableOSUpgrade = false
		}),
		inexactMatch: true,
		// We're just checking for apt-flags. We don't much care if
		// the script matches.
		expectScripts: "",
		setEnvConfig:  true,
	},
	// Test that cloudinit respects update/upgrade settings.
	{
		cfg: minimalMachineConfig(func(mc cloudinit.MachineConfig) {
			mc.EnableOSRefreshUpdate = true
			mc.EnableOSUpgrade = false
		}),
		inexactMatch: true,
		// We're just checking for apt-flags. We don't much care if
		// the script matches.
		expectScripts: "",
		setEnvConfig:  true,
	},
	// Test that cloudinit respects update/upgrade settings.
	{
		cfg: minimalMachineConfig(func(mc cloudinit.MachineConfig) {
			mc.EnableOSRefreshUpdate = false
			mc.EnableOSUpgrade = true
		}),
		inexactMatch: true,
		// We're just checking for apt-flags. We don't much care if
		// the script matches.
		expectScripts: "",
		setEnvConfig:  true,
	},
	{
		// precise state server
		cfg: cloudinit.MachineConfig{
			MachineId:        "0",
			AuthorizedKeys:   "sshkey1",
			AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
			// precise currently needs mongo from PPA
			Tools:            newSimpleTools("1.2.3-precise-amd64"),
			Series:           "precise",
			Bootstrap:        true,
			StateServingInfo: stateServingInfo,
			MachineNonce:     "FAKE_NONCE",
			MongoInfo: &mongo.MongoInfo{
				Password: "arble",
				Info: mongo.Info{
					CACert: "CA CERT\n" + testing.CACert,
				},
			},
			APIInfo: &api.Info{
				Password: "bletch",
				CACert:   "CA CERT\n" + testing.CACert,
			},
			Constraints:             envConstraints,
			DataDir:                 environs.DataDir,
			LogDir:                  agent.DefaultLogDir,
			Jobs:                    allMachineJobs,
			CloudInitOutputLog:      cloudInitOutputLog,
			InstanceId:              "i-bootstrap",
			MachineAgentServiceName: "jujud-machine-0",
			EnableOSRefreshUpdate:   true,
		},
		setEnvConfig: true,
		expectScripts: `
install -D -m 644 /dev/null '/etc/apt/preferences\.d/50-cloud-tools'
printf '%s\\n' '.*' > '/etc/apt/preferences\.d/50-cloud-tools'
set -xe
install -D -m 644 /dev/null '/var/lib/juju/nonce.txt'
printf '%s\\n' 'FAKE_NONCE' > '/var/lib/juju/nonce.txt'
test -e /proc/self/fd/9 \|\| exec 9>&2
\(\[ ! -e /home/ubuntu/.profile \] \|\| grep -q '.juju-proxy' /home/ubuntu/.profile\) \|\| printf .* >> /home/ubuntu/.profile
mkdir -p /var/lib/juju/locks
\[ -e /home/ubuntu \] && chown ubuntu:ubuntu /var/lib/juju/locks
mkdir -p /var/log/juju
chown syslog:adm /var/log/juju
bin='/var/lib/juju/tools/1\.2\.3-precise-amd64'
mkdir -p \$bin
echo 'Fetching tools.*
curl .* -o \$bin/tools\.tar\.gz 'http://foo\.com/tools/releases/juju1\.2\.3-precise-amd64\.tgz'
sha256sum \$bin/tools\.tar\.gz > \$bin/juju1\.2\.3-precise-amd64\.sha256
grep '1234' \$bin/juju1\.2\.3-precise-amd64.sha256 \|\| \(echo "Tools checksum mismatch"; exit 1\)
tar zxf \$bin/tools.tar.gz -C \$bin
printf %s '{"version":"1\.2\.3-precise-amd64","url":"http://foo\.com/tools/releases/juju1\.2\.3-precise-amd64\.tgz","sha256":"1234","size":10}' > \$bin/downloaded-tools\.txt
mkdir -p '/var/lib/juju/agents/machine-0'
install -m 600 /dev/null '/var/lib/juju/agents/machine-0/agent\.conf'
printf '%s\\n' '.*' > '/var/lib/juju/agents/machine-0/agent\.conf'
echo 'Bootstrapping Juju machine agent'.*
/var/lib/juju/tools/1\.2\.3-precise-amd64/jujud bootstrap-state --data-dir '/var/lib/juju' --env-config '[^']*' --instance-id 'i-bootstrap' --constraints 'mem=2048M' --debug
ln -s 1\.2\.3-precise-amd64 '/var/lib/juju/tools/machine-0'
echo 'Starting Juju machine agent \(jujud-machine-0\)'.*
cat >> /etc/init/jujud-machine-0\.conf << 'EOF'\\ndescription "juju machine-0 agent"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\n\\nlimit nofile 20000 20000\\n\\nscript\\n\\n  # Ensure log files are properly protected\\n  touch /var/log/juju/machine-0\.log\\n  chown syslog:syslog /var/log/juju/machine-0\.log\\n  chmod 0600 /var/log/juju/machine-0\.log\\n\\n  exec /var/lib/juju/tools/machine-0/jujud machine --data-dir '/var/lib/juju' --machine-id 0 --debug >> /var/log/juju/machine-0\.log 2>&1\\nend script\\nEOF\\n
start jujud-machine-0
rm \$bin/tools\.tar\.gz && rm \$bin/juju1\.2\.3-precise-amd64\.sha256
`,
	}, {
		// raring state server - we just test the raring-specific parts of the output.
		cfg: cloudinit.MachineConfig{
			MachineId:        "0",
			AuthorizedKeys:   "sshkey1",
			AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
			// raring provides mongo in the archive
			Tools:            newSimpleTools("1.2.3-raring-amd64"),
			Series:           "raring",
			Bootstrap:        true,
			StateServingInfo: stateServingInfo,
			MachineNonce:     "FAKE_NONCE",
			MongoInfo: &mongo.MongoInfo{
				Password: "arble",
				Info: mongo.Info{
					CACert: "CA CERT\n" + testing.CACert,
				},
			},
			APIInfo: &api.Info{
				Password: "bletch",
				CACert:   "CA CERT\n" + testing.CACert,
			},
			Constraints:             envConstraints,
			DataDir:                 environs.DataDir,
			LogDir:                  agent.DefaultLogDir,
			Jobs:                    allMachineJobs,
			CloudInitOutputLog:      cloudInitOutputLog,
			InstanceId:              "i-bootstrap",
			MachineAgentServiceName: "jujud-machine-0",
			EnableOSRefreshUpdate:   true,
		},
		setEnvConfig: true,
		inexactMatch: true,
		expectScripts: `
bin='/var/lib/juju/tools/1\.2\.3-raring-amd64'
curl .* -o \$bin/tools\.tar\.gz 'http://foo\.com/tools/releases/juju1\.2\.3-raring-amd64\.tgz'
sha256sum \$bin/tools\.tar\.gz > \$bin/juju1\.2\.3-raring-amd64\.sha256
grep '1234' \$bin/juju1\.2\.3-raring-amd64.sha256 \|\| \(echo "Tools checksum mismatch"; exit 1\)
printf %s '{"version":"1\.2\.3-raring-amd64","url":"http://foo\.com/tools/releases/juju1\.2\.3-raring-amd64\.tgz","sha256":"1234","size":10}' > \$bin/downloaded-tools\.txt
/var/lib/juju/tools/1\.2\.3-raring-amd64/jujud bootstrap-state --data-dir '/var/lib/juju' --env-config '[^']*' --instance-id 'i-bootstrap' --constraints 'mem=2048M' --debug
ln -s 1\.2\.3-raring-amd64 '/var/lib/juju/tools/machine-0'
rm \$bin/tools\.tar\.gz && rm \$bin/juju1\.2\.3-raring-amd64\.sha256
`,
	}, {
		// non state server.
		cfg: cloudinit.MachineConfig{
			MachineId:          "99",
			AuthorizedKeys:     "sshkey1",
			AgentEnvironment:   map[string]string{agent.ProviderType: "dummy"},
			DataDir:            environs.DataDir,
			LogDir:             agent.DefaultLogDir,
			Jobs:               normalMachineJobs,
			CloudInitOutputLog: cloudInitOutputLog,
			Bootstrap:          false,
			Tools:              newSimpleTools("1.2.3-quantal-amd64"),
			Series:             "quantal",
			MachineNonce:       "FAKE_NONCE",
			MongoInfo: &mongo.MongoInfo{
				Tag:      names.NewMachineTag("99"),
				Password: "arble",
				Info: mongo.Info{
					Addrs:  []string{"state-addr.testing.invalid:12345"},
					CACert: "CA CERT\n" + testing.CACert,
				},
			},
			APIInfo: &api.Info{
				Addrs:    []string{"state-addr.testing.invalid:54321"},
				Tag:      names.NewMachineTag("99"),
				Password: "bletch",
				CACert:   "CA CERT\n" + testing.CACert,
			},
			MachineAgentServiceName: "jujud-machine-99",
			PreferIPv6:              true,
			EnableOSRefreshUpdate:   true,
		},
		expectScripts: `
set -xe
install -D -m 644 /dev/null '/var/lib/juju/nonce.txt'
printf '%s\\n' 'FAKE_NONCE' > '/var/lib/juju/nonce.txt'
test -e /proc/self/fd/9 \|\| exec 9>&2
\(\[ ! -e /home/ubuntu/\.profile \] \|\| grep -q '.juju-proxy' /home/ubuntu/.profile\) \|\| printf .* >> /home/ubuntu/.profile
mkdir -p /var/lib/juju/locks
\[ -e /home/ubuntu \] && chown ubuntu:ubuntu /var/lib/juju/locks
mkdir -p /var/log/juju
chown syslog:adm /var/log/juju
bin='/var/lib/juju/tools/1\.2\.3-quantal-amd64'
mkdir -p \$bin
echo 'Fetching tools.*
curl .* --insecure -o \$bin/tools\.tar\.gz 'https://state-addr\.testing\.invalid:54321/tools/1\.2\.3-quantal-amd64'
sha256sum \$bin/tools\.tar\.gz > \$bin/juju1\.2\.3-quantal-amd64\.sha256
grep '1234' \$bin/juju1\.2\.3-quantal-amd64.sha256 \|\| \(echo "Tools checksum mismatch"; exit 1\)
tar zxf \$bin/tools.tar.gz -C \$bin
printf %s '{"version":"1\.2\.3-quantal-amd64","url":"http://foo\.com/tools/releases/juju1\.2\.3-quantal-amd64\.tgz","sha256":"1234","size":10}' > \$bin/downloaded-tools\.txt
mkdir -p '/var/lib/juju/agents/machine-99'
install -m 600 /dev/null '/var/lib/juju/agents/machine-99/agent\.conf'
printf '%s\\n' '.*' > '/var/lib/juju/agents/machine-99/agent\.conf'
ln -s 1\.2\.3-quantal-amd64 '/var/lib/juju/tools/machine-99'
echo 'Starting Juju machine agent \(jujud-machine-99\)'.*
cat >> /etc/init/jujud-machine-99\.conf << 'EOF'\\ndescription "juju machine-99 agent"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\n\\nlimit nofile 20000 20000\\n\\nscript\\n\\n  # Ensure log files are properly protected\\n  touch /var/log/juju/machine-99\.log\\n  chown syslog:syslog /var/log/juju/machine-99\.log\\n  chmod 0600 /var/log/juju/machine-99\.log\\n\\n  exec /var/lib/juju/tools/machine-99/jujud machine --data-dir '/var/lib/juju' --machine-id 99 --debug >> /var/log/juju/machine-99\.log 2>&1\\nend script\\nEOF\\n
start jujud-machine-99
rm \$bin/tools\.tar\.gz && rm \$bin/juju1\.2\.3-quantal-amd64\.sha256
`,
	}, {
		// check that it works ok with compound machine ids.
		cfg: cloudinit.MachineConfig{
			MachineId:            "2/lxc/1",
			MachineContainerType: "lxc",
			AuthorizedKeys:       "sshkey1",
			AgentEnvironment:     map[string]string{agent.ProviderType: "dummy"},
			DataDir:              environs.DataDir,
			LogDir:               agent.DefaultLogDir,
			Jobs:                 normalMachineJobs,
			CloudInitOutputLog:   cloudInitOutputLog,
			Bootstrap:            false,
			Tools:                newSimpleTools("1.2.3-quantal-amd64"),
			Series:               "quantal",
			MachineNonce:         "FAKE_NONCE",
			MongoInfo: &mongo.MongoInfo{
				Tag:      names.NewMachineTag("2/lxc/1"),
				Password: "arble",
				Info: mongo.Info{
					Addrs:  []string{"state-addr.testing.invalid:12345"},
					CACert: "CA CERT\n" + testing.CACert,
				},
			},
			APIInfo: &api.Info{
				Addrs:    []string{"state-addr.testing.invalid:54321"},
				Tag:      names.NewMachineTag("2/lxc/1"),
				Password: "bletch",
				CACert:   "CA CERT\n" + testing.CACert,
			},
			MachineAgentServiceName: "jujud-machine-2-lxc-1",
			EnableOSRefreshUpdate:   true,
		},
		inexactMatch: true,
		expectScripts: `
mkdir -p '/var/lib/juju/agents/machine-2-lxc-1'
install -m 600 /dev/null '/var/lib/juju/agents/machine-2-lxc-1/agent\.conf'
printf '%s\\n' '.*' > '/var/lib/juju/agents/machine-2-lxc-1/agent\.conf'
ln -s 1\.2\.3-quantal-amd64 '/var/lib/juju/tools/machine-2-lxc-1'
cat >> /etc/init/jujud-machine-2-lxc-1\.conf << 'EOF'\\ndescription "juju machine-2-lxc-1 agent"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\n\\nlimit nofile 20000 20000\\n\\nscript\\n\\n  # Ensure log files are properly protected\\n  touch /var/log/juju/machine-2-lxc-1\.log\\n  chown syslog:syslog /var/log/juju/machine-2-lxc-1\.log\\n  chmod 0600 /var/log/juju/machine-2-lxc-1\.log\\n\\n  exec /var/lib/juju/tools/machine-2-lxc-1/jujud machine --data-dir '/var/lib/juju' --machine-id 2/lxc/1 --debug >> /var/log/juju/machine-2-lxc-1\.log 2>&1\\nend script\\nEOF\\n
start jujud-machine-2-lxc-1
`,
	}, {
		// hostname verification disabled.
		cfg: cloudinit.MachineConfig{
			MachineId:          "99",
			AuthorizedKeys:     "sshkey1",
			AgentEnvironment:   map[string]string{agent.ProviderType: "dummy"},
			DataDir:            environs.DataDir,
			LogDir:             agent.DefaultLogDir,
			Jobs:               normalMachineJobs,
			CloudInitOutputLog: cloudInitOutputLog,
			Bootstrap:          false,
			Tools:              newSimpleTools("1.2.3-quantal-amd64"),
			Series:             "quantal",
			MachineNonce:       "FAKE_NONCE",
			MongoInfo: &mongo.MongoInfo{
				Tag:      names.NewMachineTag("99"),
				Password: "arble",
				Info: mongo.Info{
					Addrs:  []string{"state-addr.testing.invalid:12345"},
					CACert: "CA CERT\n" + testing.CACert,
				},
			},
			APIInfo: &api.Info{
				Addrs:    []string{"state-addr.testing.invalid:54321"},
				Tag:      names.NewMachineTag("99"),
				Password: "bletch",
				CACert:   "CA CERT\n" + testing.CACert,
			},
			DisableSSLHostnameVerification: true,
			MachineAgentServiceName:        "jujud-machine-99",
			EnableOSRefreshUpdate:          true,
		},
		inexactMatch: true,
		expectScripts: `
curl .* --insecure -o \$bin/tools\.tar\.gz 'https://state-addr\.testing\.invalid:54321/tools/1\.2\.3-quantal-amd64'
`,
	}, {
		// empty contraints.
		cfg: cloudinit.MachineConfig{
			MachineId:        "0",
			AuthorizedKeys:   "sshkey1",
			AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
			// precise currently needs mongo from PPA
			Tools:            newSimpleTools("1.2.3-precise-amd64"),
			Series:           "precise",
			Bootstrap:        true,
			StateServingInfo: stateServingInfo,
			MachineNonce:     "FAKE_NONCE",
			MongoInfo: &mongo.MongoInfo{
				Password: "arble",
				Info: mongo.Info{
					CACert: "CA CERT\n" + testing.CACert,
				},
			},
			APIInfo: &api.Info{
				Password: "bletch",
				CACert:   "CA CERT\n" + testing.CACert,
			},
			DataDir:                 environs.DataDir,
			LogDir:                  agent.DefaultLogDir,
			Jobs:                    allMachineJobs,
			CloudInitOutputLog:      cloudInitOutputLog,
			InstanceId:              "i-bootstrap",
			MachineAgentServiceName: "jujud-machine-0",
			EnableOSRefreshUpdate:   true,
		},
		setEnvConfig: true,
		inexactMatch: true,
		expectScripts: `
/var/lib/juju/tools/1\.2\.3-precise-amd64/jujud bootstrap-state --data-dir '/var/lib/juju' --env-config '[^']*' --instance-id 'i-bootstrap' --debug
`,
	}, {
		// custom image metadata.
		cfg: cloudinit.MachineConfig{
			MachineId:        "0",
			AuthorizedKeys:   "sshkey1",
			AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
			// precise currently needs mongo from PPA
			Tools:            newSimpleTools("1.2.3-precise-amd64"),
			Series:           "precise",
			Bootstrap:        true,
			StateServingInfo: stateServingInfo,
			MachineNonce:     "FAKE_NONCE",
			MongoInfo: &mongo.MongoInfo{
				Password: "arble",
				Info: mongo.Info{
					CACert: "CA CERT\n" + testing.CACert,
				},
			},
			APIInfo: &api.Info{
				Password: "bletch",
				CACert:   "CA CERT\n" + testing.CACert,
			},
			DataDir:                 environs.DataDir,
			LogDir:                  agent.DefaultLogDir,
			Jobs:                    allMachineJobs,
			CloudInitOutputLog:      cloudInitOutputLog,
			InstanceId:              "i-bootstrap",
			MachineAgentServiceName: "jujud-machine-0",
			EnableOSRefreshUpdate:   true,
			CustomImageMetadata: []*imagemetadata.ImageMetadata{&imagemetadata.ImageMetadata{
				Id:         "image-id",
				Storage:    "ebs",
				VirtType:   "pv",
				Arch:       "amd64",
				Version:    "14.04",
				RegionName: "us-east1",
			}},
		},
		setEnvConfig: true,
		inexactMatch: true,
		expectScripts: `
printf '%s\\n' '.*' > '/var/lib/juju/simplestreams/images/streams/v1/index\.json'
printf '%s\\n' '.*' > '/var/lib/juju/simplestreams/images/streams/v1/com.ubuntu.cloud:released:imagemetadata\.json'
`,
	},
}

func newSimpleTools(vers string) *tools.Tools {
	return &tools.Tools{
		URL:     "http://foo.com/tools/releases/juju" + vers + ".tgz",
		Version: version.MustParseBinary(vers),
		Size:    10,
		SHA256:  "1234",
	}
}

func newFileTools(vers, path string) *tools.Tools {
	tools := newSimpleTools(vers)
	tools.URL = "file://" + path
	return tools
}

func getAgentConfig(c *gc.C, tag string, scripts []string) (cfg string) {
	re := regexp.MustCompile(`printf '%s\\n' '((\n|.)+)' > .*agents/` + regexp.QuoteMeta(tag) + `/agent\.conf`)
	found := false
	for _, s := range scripts {
		m := re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		cfg = m[1]
		found = true
	}
	c.Assert(found, gc.Equals, true)
	return cfg
}

// check that any --env-config $base64 is valid and matches t.cfg.Config
func checkEnvConfig(c *gc.C, cfg *config.Config, x map[interface{}]interface{}, scripts []string) {
	re := regexp.MustCompile(`--env-config '([^']+)'`)
	found := false
	for _, s := range scripts {
		m := re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		found = true
		buf, err := base64.StdEncoding.DecodeString(m[1])
		c.Assert(err, gc.IsNil)
		var actual map[string]interface{}
		err = goyaml.Unmarshal(buf, &actual)
		c.Assert(err, gc.IsNil)
		c.Assert(cfg.AllAttrs(), jc.DeepEquals, actual)
	}
	c.Assert(found, gc.Equals, true)
}

// TestCloudInit checks that the output from the various tests
// in cloudinitTests is well formed.
func (*cloudinitSuite) TestCloudInit(c *gc.C) {
	for i, test := range cloudinitTests {

		c.Logf("test %d", i)
		if test.setEnvConfig {
			test.cfg.Config = minimalConfig(c)
		}
		ci := coreCloudinit.New()
		udata, err := cloudinit.NewUserdataConfig(&test.cfg, ci)
		c.Assert(err, gc.IsNil)
		err = udata.Configure()

		c.Assert(err, gc.IsNil)
		c.Check(ci, gc.NotNil)
		// render the cloudinit config to bytes, and then
		// back to a map so we can introspect it without
		// worrying about internal details of the cloudinit
		// package.
		data, err := udata.Render()
		c.Assert(err, gc.IsNil)

		configKeyValues := make(map[interface{}]interface{})
		err = goyaml.Unmarshal(data, &configKeyValues)
		c.Assert(err, gc.IsNil)

		c.Check(configKeyValues["apt_get_wrapper"], gc.DeepEquals, map[interface{}]interface{}{
			"command": "eatmydata",
			"enabled": "auto",
		})

		if test.cfg.EnableOSRefreshUpdate {
			c.Check(configKeyValues["apt_update"], gc.Equals, true)
		} else {
			c.Check(configKeyValues["apt_update"], gc.IsNil)
		}

		if test.cfg.EnableOSUpgrade {
			c.Check(configKeyValues["apt_upgrade"], gc.Equals, true)
		} else {
			c.Check(configKeyValues["apt_upgrade"], gc.IsNil)
		}

		scripts := getScripts(configKeyValues)
		assertScriptMatch(c, scripts, test.expectScripts, !test.inexactMatch)
		if test.cfg.Config != nil {
			checkEnvConfig(c, test.cfg.Config, configKeyValues, scripts)
		}
		checkPackage(c, configKeyValues, "curl", test.cfg.EnableOSRefreshUpdate)

		tag := names.NewMachineTag(test.cfg.MachineId).String()
		acfg := getAgentConfig(c, tag, scripts)
		c.Assert(acfg, jc.Contains, "AGENT_SERVICE_NAME: jujud-"+tag)
		c.Assert(acfg, jc.Contains, "upgradedToVersion: 1.2.3\n")
		source := "deb http://ubuntu-cloud.archive.canonical.com/ubuntu precise-updates/cloud-tools main"
		needCloudArchive := test.cfg.Series == "precise"
		checkAptSource(c, configKeyValues, source, cloudinit.CanonicalCloudArchiveSigningKey, needCloudArchive)
	}
}

func (*cloudinitSuite) TestCloudInitConfigure(c *gc.C) {
	for i, test := range cloudinitTests {
		test.cfg.Config = minimalConfig(c)
		c.Logf("test %d (Configure)", i)
		cloudcfg := coreCloudinit.New()
		udata, err := cloudinit.NewUserdataConfig(&test.cfg, cloudcfg)
		c.Assert(err, gc.IsNil)
		err = udata.Configure()
		c.Assert(err, gc.IsNil)
	}
}

func (*cloudinitSuite) TestCloudInitConfigureUsesGivenConfig(c *gc.C) {
	// Create a simple cloudinit config with a 'runcmd' statement.
	cloudcfg := coreCloudinit.New()
	script := "test script"
	cloudcfg.AddRunCmd(script)
	cloudinitTests[0].cfg.Config = minimalConfig(c)
	udata, err := cloudinit.NewUserdataConfig(&cloudinitTests[0].cfg, cloudcfg)
	c.Assert(err, gc.IsNil)
	err = udata.Configure()
	c.Assert(err, gc.IsNil)
	data, err := udata.Render()
	c.Assert(err, gc.IsNil)

	ciContent := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, &ciContent)
	c.Assert(err, gc.IsNil)
	// The 'runcmd' statement is at the beginning of the list
	// of 'runcmd' statements.
	runCmd := ciContent["runcmd"].([]interface{})
	c.Check(runCmd[0], gc.Equals, script)
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

func assertScriptMatch(c *gc.C, got []string, expect string, exact bool) {

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
			c.Fatalf("could not find match for %q", pats[0].line)
		default:
			ok, err := regexp.MatchString(pats[0].line, scripts[0].line)
			c.Assert(err, gc.IsNil)
			if ok {
				pats = pats[1:]
				scripts = scripts[1:]
			} else if exact {
				c.Assert(scripts[0].line, gc.Matches, pats[0].line, gc.Commentf("line %d", scripts[0].index))
			} else {
				scripts = scripts[1:]
			}
		}
	}
}

// checkPackage checks that the cloudinit will or won't install the given
// package, depending on the value of match.
func checkPackage(c *gc.C, x map[interface{}]interface{}, pkg string, match bool) {
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
		hasTargetRelease := strings.Contains(p, "--target-release")
		hasQuotedPkg := strings.Contains(p, "'"+pkg+"'")
		if p == pkg || (hasTargetRelease && hasQuotedPkg) {
			found = true
		}
	}
	switch {
	case match && !found:
		c.Errorf("package %q not found in %v", pkg, pkgs)
	case !match && found:
		c.Errorf("%q found but not expected in %v", pkg, pkgs)
	}
}

// checkAptSource checks that the cloudinit will or won't install the given
// source, depending on the value of match.
func checkAptSource(c *gc.C, x map[interface{}]interface{}, source, key string, match bool) {
	sources0 := x["apt_sources"]
	if sources0 == nil {
		if match {
			c.Errorf("cloudinit has no entry for apt_sources")
		}
		return
	}

	sources := sources0.([]interface{})

	found := false
	for _, s0 := range sources {
		s := s0.(map[interface{}]interface{})
		if s["source"] == source && s["key"] == key {
			found = true
		}
	}
	switch {
	case match && !found:
		c.Errorf("source %q not found in %v", source, sources)
	case !match && found:
		c.Errorf("%q found but not expected in %v", source, sources)
	}
}

// When mutate is called on a known-good MachineConfig,
// there should be an error complaining about the missing
// field named by the adjacent err.
var verifyTests = []struct {
	err    string
	mutate func(*cloudinit.MachineConfig)
}{
	{"invalid machine id", func(cfg *cloudinit.MachineConfig) {
		cfg.MachineId = "-1"
	}},
	{"missing environment configuration", func(cfg *cloudinit.MachineConfig) {
		cfg.Config = nil
	}},
	{"missing state info", func(cfg *cloudinit.MachineConfig) {
		cfg.MongoInfo = nil
	}},
	{"missing API info", func(cfg *cloudinit.MachineConfig) {
		cfg.APIInfo = nil
	}},
	{"missing state hosts", func(cfg *cloudinit.MachineConfig) {
		cfg.Bootstrap = false
		cfg.MongoInfo = &mongo.MongoInfo{
			Tag: names.NewMachineTag("99"),
			Info: mongo.Info{
				CACert: testing.CACert,
			},
		}
		cfg.APIInfo = &api.Info{
			Addrs:  []string{"foo:35"},
			Tag:    names.NewMachineTag("99"),
			CACert: testing.CACert,
		}
	}},
	{"missing API hosts", func(cfg *cloudinit.MachineConfig) {
		cfg.Bootstrap = false
		cfg.MongoInfo = &mongo.MongoInfo{
			Info: mongo.Info{
				Addrs:  []string{"foo:35"},
				CACert: testing.CACert,
			},
			Tag: names.NewMachineTag("99"),
		}
		cfg.APIInfo = &api.Info{
			Tag:    names.NewMachineTag("99"),
			CACert: testing.CACert,
		}
	}},
	{"missing CA certificate", func(cfg *cloudinit.MachineConfig) {
		cfg.MongoInfo = &mongo.MongoInfo{Info: mongo.Info{Addrs: []string{"host:98765"}}}
	}},
	{"missing CA certificate", func(cfg *cloudinit.MachineConfig) {
		cfg.Bootstrap = false
		cfg.MongoInfo = &mongo.MongoInfo{
			Tag: names.NewMachineTag("99"),
			Info: mongo.Info{
				Addrs: []string{"host:98765"},
			},
		}
	}},
	{"missing state server certificate", func(cfg *cloudinit.MachineConfig) {
		info := *cfg.StateServingInfo
		info.Cert = ""
		cfg.StateServingInfo = &info
	}},
	{"missing state server private key", func(cfg *cloudinit.MachineConfig) {
		info := *cfg.StateServingInfo
		info.PrivateKey = ""
		cfg.StateServingInfo = &info
	}},
	{"missing state port", func(cfg *cloudinit.MachineConfig) {
		info := *cfg.StateServingInfo
		info.StatePort = 0
		cfg.StateServingInfo = &info
	}},
	{"missing API port", func(cfg *cloudinit.MachineConfig) {
		info := *cfg.StateServingInfo
		info.APIPort = 0
		cfg.StateServingInfo = &info
	}},
	{"missing var directory", func(cfg *cloudinit.MachineConfig) {
		cfg.DataDir = ""
	}},
	{"missing log directory", func(cfg *cloudinit.MachineConfig) {
		cfg.LogDir = ""
	}},
	{"missing cloud-init output log path", func(cfg *cloudinit.MachineConfig) {
		cfg.CloudInitOutputLog = ""
	}},
	{"missing tools", func(cfg *cloudinit.MachineConfig) {
		cfg.Tools = nil
	}},
	{"missing tools URL", func(cfg *cloudinit.MachineConfig) {
		cfg.Tools = &tools.Tools{}
	}},
	{"entity tag must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.Bootstrap = false
		info := *cfg.MongoInfo
		info.Tag = names.NewMachineTag("0")
		cfg.MongoInfo = &info
	}},
	{"entity tag must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.Bootstrap = false
		info := *cfg.MongoInfo
		info.Tag = nil // admin user
		cfg.MongoInfo = &info
	}},
	{"entity tag must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.Bootstrap = false
		info := *cfg.APIInfo
		info.Tag = names.NewMachineTag("0")
		cfg.APIInfo = &info
	}},
	{"entity tag must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.Bootstrap = false
		info := *cfg.APIInfo
		info.Tag = nil
		cfg.APIInfo = &info
	}},
	{"entity tag must be nil when starting a state server", func(cfg *cloudinit.MachineConfig) {
		info := *cfg.MongoInfo
		info.Tag = names.NewMachineTag("0")
		cfg.MongoInfo = &info
	}},
	{"entity tag must be nil when starting a state server", func(cfg *cloudinit.MachineConfig) {
		info := *cfg.APIInfo
		info.Tag = names.NewMachineTag("0")
		cfg.APIInfo = &info
	}},
	{"missing machine nonce", func(cfg *cloudinit.MachineConfig) {
		cfg.MachineNonce = ""
	}},
	{"missing machine agent service name", func(cfg *cloudinit.MachineConfig) {
		cfg.MachineAgentServiceName = ""
	}},
	{"missing instance-id", func(cfg *cloudinit.MachineConfig) {
		cfg.InstanceId = ""
	}},
	{"state serving info unexpectedly present", func(cfg *cloudinit.MachineConfig) {
		cfg.Bootstrap = false
		apiInfo := *cfg.APIInfo
		apiInfo.Tag = names.NewMachineTag("99")
		cfg.APIInfo = &apiInfo
		stateInfo := *cfg.MongoInfo
		stateInfo.Tag = names.NewMachineTag("99")
		cfg.MongoInfo = &stateInfo
	}},
}

// TestCloudInitVerify checks that required fields are appropriately
// checked for by NewCloudInit.
func (*cloudinitSuite) TestCloudInitVerify(c *gc.C) {
	cfg := &cloudinit.MachineConfig{
		Bootstrap:        true,
		StateServingInfo: stateServingInfo,
		MachineId:        "99",
		Tools:            newSimpleTools("9.9.9-quantal-arble"),
		AuthorizedKeys:   "sshkey1",
		Series:           "quantal",
		AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
		MongoInfo: &mongo.MongoInfo{
			Info: mongo.Info{
				Addrs:  []string{"host:98765"},
				CACert: testing.CACert,
			},
			Password: "password",
		},
		APIInfo: &api.Info{
			Addrs:  []string{"host:9999"},
			CACert: testing.CACert,
		},
		Config:                  minimalConfig(c),
		DataDir:                 environs.DataDir,
		LogDir:                  agent.DefaultLogDir,
		Jobs:                    normalMachineJobs,
		CloudInitOutputLog:      cloudInitOutputLog,
		InstanceId:              "i-bootstrap",
		MachineNonce:            "FAKE_NONCE",
		MachineAgentServiceName: "jujud-machine-99",
	}
	// check that the base configuration does not give an error
	ci := coreCloudinit.New()

	for i, test := range verifyTests {
		// check that the base configuration does not give an error
		// and that a previous test hasn't mutated it accidentially.
		udata, err := cloudinit.NewUserdataConfig(cfg, ci)
		c.Assert(err, gc.IsNil)
		err = udata.Configure()
		c.Assert(err, gc.IsNil)

		c.Logf("test %d. %s", i, test.err)

		cfg1 := *cfg
		test.mutate(&cfg1)

		udata, err = cloudinit.NewUserdataConfig(&cfg1, ci)
		c.Assert(err, gc.IsNil)
		err = udata.Configure()
		c.Check(err, gc.ErrorMatches, "invalid machine configuration: "+test.err)

	}
}

func (*cloudinitSuite) createMachineConfig(c *gc.C, environConfig *config.Config) *cloudinit.MachineConfig {
	machineId := "42"
	machineNonce := "fake-nonce"
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	machineConfig, err := environs.NewMachineConfig(machineId, machineNonce, imagemetadata.ReleasedStream, "quantal", nil, stateInfo, apiInfo)
	c.Assert(err, gc.IsNil)
	machineConfig.Tools = &tools.Tools{
		Version: version.MustParseBinary("2.3.4-quantal-amd64"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-amd64.tgz",
	}
	err = environs.FinishMachineConfig(machineConfig, environConfig)
	c.Assert(err, gc.IsNil)
	return machineConfig
}

func (s *cloudinitSuite) TestAptProxyNotWrittenIfNotSet(c *gc.C) {
	environConfig := minimalConfig(c)
	machineCfg := s.createMachineConfig(c, environConfig)
	cloudcfg := coreCloudinit.New()
	udata, err := cloudinit.NewUserdataConfig(machineCfg, cloudcfg)
	c.Assert(err, gc.IsNil)
	err = udata.Configure()
	c.Assert(err, gc.IsNil)

	cmds := cloudcfg.BootCmds()
	c.Assert(cmds, jc.DeepEquals, []interface{}{})
}

func (s *cloudinitSuite) TestAptProxyWritten(c *gc.C) {
	environConfig := minimalConfig(c)
	environConfig, err := environConfig.Apply(map[string]interface{}{
		"apt-http-proxy": "http://user@10.0.0.1",
	})
	c.Assert(err, gc.IsNil)
	machineCfg := s.createMachineConfig(c, environConfig)
	cloudcfg := coreCloudinit.New()
	udata, err := cloudinit.NewUserdataConfig(machineCfg, cloudcfg)
	c.Assert(err, gc.IsNil)
	err = udata.Configure()
	c.Assert(err, gc.IsNil)

	cmds := cloudcfg.BootCmds()
	expected := "[ -f /etc/apt/apt.conf.d/42-juju-proxy-settings ] || (printf '%s\\n' 'Acquire::http::Proxy \"http://user@10.0.0.1\";' > /etc/apt/apt.conf.d/42-juju-proxy-settings)"
	c.Assert(cmds, jc.DeepEquals, []interface{}{expected})
}

func (s *cloudinitSuite) TestProxyWritten(c *gc.C) {
	environConfig := minimalConfig(c)
	environConfig, err := environConfig.Apply(map[string]interface{}{
		"http-proxy": "http://user@10.0.0.1",
		"no-proxy":   "localhost,10.0.3.1",
	})
	c.Assert(err, gc.IsNil)
	machineCfg := s.createMachineConfig(c, environConfig)
	cloudcfg := coreCloudinit.New()
	udata, err := cloudinit.NewUserdataConfig(machineCfg, cloudcfg)
	c.Assert(err, gc.IsNil)
	err = udata.Configure()
	c.Assert(err, gc.IsNil)

	cmds := cloudcfg.RunCmds()
	first := `([ ! -e /home/ubuntu/.profile ] || grep -q '.juju-proxy' /home/ubuntu/.profile) || printf '\n# Added by juju\n[ -f "$HOME/.juju-proxy" ] && . "$HOME/.juju-proxy"\n' >> /home/ubuntu/.profile`
	expected := []interface{}{
		`export http_proxy=http://user@10.0.0.1`,
		`export HTTP_PROXY=http://user@10.0.0.1`,
		`export no_proxy=localhost,10.0.3.1`,
		`export NO_PROXY=localhost,10.0.3.1`,
		`[ -e /home/ubuntu ] && (printf '%s\n' 'export http_proxy=http://user@10.0.0.1
export HTTP_PROXY=http://user@10.0.0.1
export no_proxy=localhost,10.0.3.1
export NO_PROXY=localhost,10.0.3.1' > /home/ubuntu/.juju-proxy && chown ubuntu:ubuntu /home/ubuntu/.juju-proxy)`,
	}
	found := false
	for i, cmd := range cmds {
		if cmd == first {
			c.Assert(cmds[i+1:i+6], jc.DeepEquals, expected)
			found = true
			break
		}
	}
	c.Assert(found, jc.IsTrue)
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

var windowsCloudinitTests = []cloudinitTest{
	{
		cfg: cloudinit.MachineConfig{
			MachineId:          "10",
			AgentEnvironment:   map[string]string{agent.ProviderType: "dummy"},
			Tools:              newSimpleTools("1.2.3-win8-amd64"),
			Series:             "win8",
			Bootstrap:          false,
			Jobs:               normalMachineJobs,
			MachineNonce:       "FAKE_NONCE",
			CloudInitOutputLog: cloudInitOutputLog,
			MongoInfo: &mongo.MongoInfo{
				Tag:      names.NewMachineTag("10"),
				Password: "arble",
				Info: mongo.Info{
					CACert: "CA CERT\n" + string(serverCert),
					Addrs:  []string{"state-addr.testing.invalid:12345"},
				},
			},
			APIInfo: &api.Info{
				Addrs:    []string{"state-addr.testing.invalid:54321"},
				Password: "bletch",
				CACert:   "CA CERT\n" + string(serverCert),
				Tag:      names.NewMachineTag("10"),
			},
			MachineAgentServiceName: "jujud-machine-0",
		},
		setEnvConfig:  false,
		expectScripts: WindowsUserdata,
	},
}

func (*cloudinitSuite) TestWindowsCloudInit(c *gc.C) {
	for i, test := range windowsCloudinitTests {
		c.Logf("test %d", i)
		dataDir, err := paths.DataDir(test.cfg.Series)
		c.Assert(err, gc.IsNil)
		logDir, err := paths.LogDir(test.cfg.Series)
		c.Assert(err, gc.IsNil)

		test.cfg.DataDir = dataDir
		test.cfg.LogDir = path.Join(logDir, "juju")

		ci := coreCloudinit.New()
		udata, err := cloudinit.NewUserdataConfig(&test.cfg, ci)

		c.Assert(err, gc.IsNil)
		err = udata.Configure()

		c.Assert(err, gc.IsNil)
		c.Check(ci, gc.NotNil)
		data, err := udata.Render()
		c.Assert(err, gc.IsNil)

		stringData := strings.Replace(string(data), "\r\n", "\n", -1)
		stringData = strings.Replace(stringData, "\t", " ", -1)
		stringData = strings.TrimSpace(stringData)

		compareString := strings.Replace(string(test.expectScripts), "\r\n", "\n", -1)
		compareString = strings.Replace(compareString, "\t", " ", -1)
		compareString = strings.TrimSpace(compareString)

		c.Assert(stringData, gc.Equals, compareString)

	}
}

func (*cloudinitSuite) TestToolsDownloadCommand(c *gc.C) {
	command := cloudinit.ToolsDownloadCommand("download", []string{"a", "b", "c"})

	expected := `
for n in $(seq 5); do

    printf "Attempt $n to download tools from %s...\n" 'a'
    download 'a' && echo "Tools downloaded successfully." && break

    printf "Attempt $n to download tools from %s...\n" 'b'
    download 'b' && echo "Tools downloaded successfully." && break

    printf "Attempt $n to download tools from %s...\n" 'c'
    download 'c' && echo "Tools downloaded successfully." && break

    if [ $n -lt 5 ]; then
        echo "Download failed..... wait 15s"
    fi
    sleep 15
done`
	c.Assert(command, gc.Equals, expected)
}
