// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"encoding/base64"
	"regexp"
	"strings"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent"
	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

// Use local suite since this file lives in the ec2 package
// for testing internals.
type cloudinitSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&cloudinitSuite{})

var envConstraints = constraints.MustParse("mem=2G")

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

func minimalConfig(c *gc.C) *config.Config {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig())
	c.Assert(err, gc.IsNil)
	return cfg
}

// Each test gives a cloudinit config - we check the
// output to see if it looks correct.
var cloudinitTests = []cloudinitTest{
	{
		// precise state server
		cfg: cloudinit.MachineConfig{
			MachineId:        "0",
			AuthorizedKeys:   "sshkey1",
			AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
			// precise currently needs mongo from PPA
			Tools:           newSimpleTools("1.2.3-precise-amd64"),
			StateServer:     true,
			StateServerCert: serverCert,
			StateServerKey:  serverKey,
			StatePort:       37017,
			APIPort:         17070,
			SyslogPort:      514,
			MachineNonce:    "FAKE_NONCE",
			StateInfo: &state.Info{
				Password: "arble",
				CACert:   []byte("CA CERT\n" + testing.CACert),
			},
			APIInfo: &api.Info{
				Password: "bletch",
				CACert:   []byte("CA CERT\n" + testing.CACert),
			},
			Constraints:  envConstraints,
			DataDir:      environs.DataDir,
			StateInfoURL: "some-url",
		},
		setEnvConfig: true,
		expectScripts: `
echo ENABLE_MONGODB="no" > /etc/default/mongodb
set -xe
mkdir -p /var/lib/juju
mkdir -p /var/log/juju
bin='/var/lib/juju/tools/1\.2\.3-precise-amd64'
mkdir -p \$bin
wget --no-verbose -O \$bin/tools\.tar\.gz 'http://foo\.com/tools/releases/juju1\.2\.3-precise-amd64\.tgz'
sha256sum \$bin/tools\.tar\.gz > \$bin/juju1\.2\.3-precise-amd64\.sha256
grep '1234' \$bin/juju1\.2\.3-precise-amd64.sha256 \|\| \(echo "Tools checksum mismatch"; exit 1\)
tar zxf \$bin/tools.tar.gz -C \$bin
rm \$bin/tools\.tar\.gz && rm \$bin/juju1\.2\.3-precise-amd64\.sha256
printf %s '{"version":"1\.2\.3-precise-amd64","url":"http://foo\.com/tools/releases/juju1\.2\.3-precise-amd64\.tgz","sha256":"1234","size":10}' > \$bin/downloaded-tools\.txt
install -m 600 /dev/null '/etc/rsyslog\.d/25-juju\.conf'
printf '%s\\n' '.*' > '/etc/rsyslog.d/25-juju.conf'
restart rsyslog
mkdir -p '/var/lib/juju/agents/machine-0'
install -m 644 /dev/null '/var/lib/juju/agents/machine-0/format'
printf '%s\\n' '.*' > '/var/lib/juju/agents/machine-0/format'
install -m 600 /dev/null '/var/lib/juju/agents/machine-0/agent\.conf'
printf '%s\\n' '.*' > '/var/lib/juju/agents/machine-0/agent\.conf'
install -m 600 /dev/null '/var/lib/juju/server\.pem'
printf '%s\\n' 'SERVER CERT\\n[^']*SERVER KEY\\n[^']*' > '/var/lib/juju/server\.pem'
mkdir -p /var/lib/juju/db/journal
chmod 0700 /var/lib/juju/db
dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc\.0
dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc\.1
dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc\.2
cat >> /etc/init/juju-db\.conf << 'EOF'\\ndescription "juju state database"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\n\\nlimit nofile 65000 65000\\nlimit nproc 20000 20000\\n\\nexec /usr/bin/mongod --auth --dbpath=/var/lib/juju/db --sslOnNormalPorts --sslPEMKeyFile '/var/lib/juju/server\.pem' --sslPEMKeyPassword ignored --bind_ip 0\.0\.0\.0 --port 37017 --noprealloc --syslog --smallfiles\\nEOF\\n
start juju-db
mkdir -p '/var/lib/juju/agents/bootstrap'
install -m 644 /dev/null '/var/lib/juju/agents/bootstrap/format'
printf '%s\\n' '.*' > '/var/lib/juju/agents/bootstrap/format'
install -m 600 /dev/null '/var/lib/juju/agents/bootstrap/agent\.conf'
printf '%s\\n' '.*' > '/var/lib/juju/agents/bootstrap/agent\.conf'
echo 'some-url' > /tmp/provider-state-url
/var/lib/juju/tools/1\.2\.3-precise-amd64/jujud bootstrap-state --data-dir '/var/lib/juju' --env-config '[^']*' --constraints 'mem=2048M' --debug
rm -rf '/var/lib/juju/agents/bootstrap'
ln -s 1\.2\.3-precise-amd64 '/var/lib/juju/tools/machine-0'
cat >> /etc/init/jujud-machine-0\.conf << 'EOF'\\ndescription "juju machine-0 agent"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\n\\nlimit nofile 20000 20000\\n\\nexec /var/lib/juju/tools/machine-0/jujud machine --data-dir '/var/lib/juju' --machine-id 0 --debug >> /var/log/juju/machine-0\.log 2>&1\\nEOF\\n
start jujud-machine-0
`,
	}, {
		// raring state server - we just test the raring-specific parts of the output.
		cfg: cloudinit.MachineConfig{
			MachineId:        "0",
			AuthorizedKeys:   "sshkey1",
			AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
			// raring provides mongo in the archive
			Tools:           newSimpleTools("1.2.3-raring-amd64"),
			StateServer:     true,
			StateServerCert: serverCert,
			StateServerKey:  serverKey,
			StatePort:       37017,
			APIPort:         17070,
			SyslogPort:      514,
			MachineNonce:    "FAKE_NONCE",
			StateInfo: &state.Info{
				Password: "arble",
				CACert:   []byte("CA CERT\n" + testing.CACert),
			},
			APIInfo: &api.Info{
				Password: "bletch",
				CACert:   []byte("CA CERT\n" + testing.CACert),
			},
			Constraints:  envConstraints,
			DataDir:      environs.DataDir,
			StateInfoURL: "some-url",
		},
		setEnvConfig: true,
		inexactMatch: true,
		expectScripts: `
bin='/var/lib/juju/tools/1\.2\.3-raring-amd64'
wget --no-verbose -O \$bin/tools\.tar\.gz 'http://foo\.com/tools/releases/juju1\.2\.3-raring-amd64\.tgz'
sha256sum \$bin/tools\.tar\.gz > \$bin/juju1\.2\.3-raring-amd64\.sha256
grep '1234' \$bin/juju1\.2\.3-raring-amd64.sha256 \|\| \(echo "Tools checksum mismatch"; exit 1\)
rm \$bin/tools\.tar\.gz && rm \$bin/juju1\.2\.3-raring-amd64\.sha256
printf %s '{"version":"1\.2\.3-raring-amd64","url":"http://foo\.com/tools/releases/juju1\.2\.3-raring-amd64\.tgz","sha256":"1234","size":10}' > \$bin/downloaded-tools\.txt
/var/lib/juju/tools/1\.2\.3-raring-amd64/jujud bootstrap-state --data-dir '/var/lib/juju' --env-config '[^']*' --constraints 'mem=2048M' --debug
rm -rf '/var/lib/juju/agents/bootstrap'
ln -s 1\.2\.3-raring-amd64 '/var/lib/juju/tools/machine-0'
`,
	}, {
		// non state server.
		cfg: cloudinit.MachineConfig{
			MachineId:        "99",
			AuthorizedKeys:   "sshkey1",
			AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
			DataDir:          environs.DataDir,
			StateServer:      false,
			Tools:            newSimpleTools("1.2.3-linux-amd64"),
			MachineNonce:     "FAKE_NONCE",
			StateInfo: &state.Info{
				Addrs:    []string{"state-addr.testing.invalid:12345"},
				Tag:      "machine-99",
				Password: "arble",
				CACert:   []byte("CA CERT\n" + testing.CACert),
			},
			APIInfo: &api.Info{
				Addrs:    []string{"state-addr.testing.invalid:54321"},
				Tag:      "machine-99",
				Password: "bletch",
				CACert:   []byte("CA CERT\n" + testing.CACert),
			},
			SyslogPort: 514,
		},
		expectScripts: `
set -xe
mkdir -p /var/lib/juju
mkdir -p /var/log/juju
bin='/var/lib/juju/tools/1\.2\.3-linux-amd64'
mkdir -p \$bin
wget --no-verbose -O \$bin/tools\.tar\.gz 'http://foo\.com/tools/releases/juju1\.2\.3-linux-amd64\.tgz'
sha256sum \$bin/tools\.tar\.gz > \$bin/juju1\.2\.3-linux-amd64\.sha256
grep '1234' \$bin/juju1\.2\.3-linux-amd64.sha256 \|\| \(echo "Tools checksum mismatch"; exit 1\)
tar zxf \$bin/tools.tar.gz -C \$bin
rm \$bin/tools\.tar\.gz && rm \$bin/juju1\.2\.3-linux-amd64\.sha256
printf %s '{"version":"1\.2\.3-linux-amd64","url":"http://foo\.com/tools/releases/juju1\.2\.3-linux-amd64\.tgz","sha256":"1234","size":10}' > \$bin/downloaded-tools\.txt
install -m 600 /dev/null '/etc/rsyslog\.d/25-juju\.conf'
printf '%s\\n' '.*' > '/etc/rsyslog\.d/25-juju\.conf'
restart rsyslog
mkdir -p '/var/lib/juju/agents/machine-99'
install -m 644 /dev/null '/var/lib/juju/agents/machine-99/format'
printf '%s\\n' '.*' > '/var/lib/juju/agents/machine-99/format'
install -m 600 /dev/null '/var/lib/juju/agents/machine-99/agent\.conf'
printf '%s\\n' '.*' > '/var/lib/juju/agents/machine-99/agent\.conf'
ln -s 1\.2\.3-linux-amd64 '/var/lib/juju/tools/machine-99'
cat >> /etc/init/jujud-machine-99\.conf << 'EOF'\\ndescription "juju machine-99 agent"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\n\\nlimit nofile 20000 20000\\n\\nexec /var/lib/juju/tools/machine-99/jujud machine --data-dir '/var/lib/juju' --machine-id 99 --debug >> /var/log/juju/machine-99\.log 2>&1\\nEOF\\n
start jujud-machine-99
`,
	}, {
		// check that it works ok with compound machine ids.
		cfg: cloudinit.MachineConfig{
			MachineId:            "2/lxc/1",
			MachineContainerType: "lxc",
			AuthorizedKeys:       "sshkey1",
			AgentEnvironment:     map[string]string{agent.ProviderType: "dummy"},
			DataDir:              environs.DataDir,
			StateServer:          false,
			Tools:                newSimpleTools("1.2.3-linux-amd64"),
			MachineNonce:         "FAKE_NONCE",
			StateInfo: &state.Info{
				Addrs:    []string{"state-addr.testing.invalid:12345"},
				Tag:      "machine-2-lxc-1",
				Password: "arble",
				CACert:   []byte("CA CERT\n" + testing.CACert),
			},
			APIInfo: &api.Info{
				Addrs:    []string{"state-addr.testing.invalid:54321"},
				Tag:      "machine-2-lxc-1",
				Password: "bletch",
				CACert:   []byte("CA CERT\n" + testing.CACert),
			},
			SyslogPort: 514,
		},
		inexactMatch: true,
		expectScripts: `
printf '%s\\n' '.*' > '/etc/rsyslog\.d/25-juju\.conf'
restart rsyslog
mkdir -p '/var/lib/juju/agents/machine-2-lxc-1'
install -m 644 /dev/null '/var/lib/juju/agents/machine-2-lxc-1/format'
printf '%s\\n' '.*' > '/var/lib/juju/agents/machine-2-lxc-1/format'
install -m 600 /dev/null '/var/lib/juju/agents/machine-2-lxc-1/agent\.conf'
printf '%s\\n' '.*' > '/var/lib/juju/agents/machine-2-lxc-1/agent\.conf'
ln -s 1\.2\.3-linux-amd64 '/var/lib/juju/tools/machine-2-lxc-1'
cat >> /etc/init/jujud-machine-2-lxc-1\.conf << 'EOF'\\ndescription "juju machine-2-lxc-1 agent"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\n\\nlimit nofile 20000 20000\\n\\nexec /var/lib/juju/tools/machine-2-lxc-1/jujud machine --data-dir '/var/lib/juju' --machine-id 2/lxc/1 --debug >> /var/log/juju/machine-2-lxc-1\.log 2>&1\\nEOF\\n
start jujud-machine-2-lxc-1
`,
	}, {
		// hostname verification disabled.
		cfg: cloudinit.MachineConfig{
			MachineId:        "99",
			AuthorizedKeys:   "sshkey1",
			AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
			DataDir:          environs.DataDir,
			StateServer:      false,
			Tools:            newSimpleTools("1.2.3-linux-amd64"),
			MachineNonce:     "FAKE_NONCE",
			StateInfo: &state.Info{
				Addrs:    []string{"state-addr.testing.invalid:12345"},
				Tag:      "machine-99",
				Password: "arble",
				CACert:   []byte("CA CERT\n" + testing.CACert),
			},
			APIInfo: &api.Info{
				Addrs:    []string{"state-addr.testing.invalid:54321"},
				Tag:      "machine-99",
				Password: "bletch",
				CACert:   []byte("CA CERT\n" + testing.CACert),
			},
			SyslogPort:                     514,
			DisableSSLHostnameVerification: true,
		},
		inexactMatch: true,
		expectScripts: `
wget --no-check-certificate --no-verbose -O \$bin/tools\.tar\.gz 'http://foo\.com/tools/releases/juju1\.2\.3-linux-amd64\.tgz'
`,
	}, {
		// empty contraints.
		cfg: cloudinit.MachineConfig{
			MachineId:        "0",
			AuthorizedKeys:   "sshkey1",
			AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
			// precise currently needs mongo from PPA
			Tools:           newSimpleTools("1.2.3-precise-amd64"),
			StateServer:     true,
			StateServerCert: serverCert,
			StateServerKey:  serverKey,
			StatePort:       37017,
			APIPort:         17070,
			SyslogPort:      514,
			MachineNonce:    "FAKE_NONCE",
			StateInfo: &state.Info{
				Password: "arble",
				CACert:   []byte("CA CERT\n" + testing.CACert),
			},
			APIInfo: &api.Info{
				Password: "bletch",
				CACert:   []byte("CA CERT\n" + testing.CACert),
			},
			DataDir:      environs.DataDir,
			StateInfoURL: "some-url",
		},
		setEnvConfig: true,
		inexactMatch: true,
		expectScripts: `
/var/lib/juju/tools/1\.2\.3-precise-amd64/jujud bootstrap-state --data-dir '/var/lib/juju' --env-config '[^']*' --debug
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
		c.Assert(cfg.AllAttrs(), gc.DeepEquals, actual)
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
		err := cloudinit.Configure(&test.cfg, ci)
		c.Assert(err, gc.IsNil)
		c.Check(ci, gc.NotNil)
		// render the cloudinit config to bytes, and then
		// back to a map so we can introspect it without
		// worrying about internal details of the cloudinit
		// package.
		data, err := ci.Render()
		c.Assert(err, gc.IsNil)

		x := make(map[interface{}]interface{})
		err = goyaml.Unmarshal(data, &x)
		c.Assert(err, gc.IsNil)

		c.Check(x["apt_upgrade"], gc.Equals, true)
		c.Check(x["apt_update"], gc.Equals, true)

		scripts := getScripts(x)
		assertScriptMatch(c, scripts, test.expectScripts, !test.inexactMatch)
		if test.cfg.Config != nil {
			checkEnvConfig(c, test.cfg.Config, x, scripts)
		}
		checkPackage(c, x, "git", true)
		if test.cfg.StateServer {
			checkPackage(c, x, "mongodb-server", true)
			source := "ppa:juju/stable"
			checkAptSource(c, x, source, "", test.cfg.NeedMongoPPA())
		}
		source := "deb http://ubuntu-cloud.archive.canonical.com/ubuntu precise-updates/cloud-tools main"
		needCloudArchive := test.cfg.Tools.Version.Series == "precise"
		checkAptSource(c, x, source, cloudinit.CanonicalCloudArchiveSigningKey, needCloudArchive)
	}
}

func (*cloudinitSuite) TestCloudInitConfigure(c *gc.C) {
	for i, test := range cloudinitTests {
		test.cfg.Config = minimalConfig(c)
		c.Logf("test %d (Configure)", i)
		cloudcfg := coreCloudinit.New()
		err := cloudinit.Configure(&test.cfg, cloudcfg)
		c.Assert(err, gc.IsNil)
	}
}

func (*cloudinitSuite) TestCloudInitConfigureUsesGivenConfig(c *gc.C) {
	// Create a simple cloudinit config with a 'runcmd' statement.
	cloudcfg := coreCloudinit.New()
	script := "test script"
	cloudcfg.AddRunCmd(script)
	cloudinitTests[0].cfg.Config = minimalConfig(c)
	err := cloudinit.Configure(&cloudinitTests[0].cfg, cloudcfg)
	c.Assert(err, gc.IsNil)
	data, err := cloudcfg.Render()
	c.Assert(err, gc.IsNil)

	ciContent := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, &ciContent)
	c.Assert(err, gc.IsNil)
	// The 'runcmd' statement is at the beginning of the list
	// of 'runcmd' statements.
	runCmd := ciContent["runcmd"].([]interface{})
	c.Check(runCmd[0], gc.Equals, script)
}

func getScripts(x map[interface{}]interface{}) []string {
	var scripts []string
	if bootcmds, ok := x["bootcmd"]; ok {
		for _, s := range bootcmds.([]interface{}) {
			scripts = append(scripts, s.(string))
		}
	}
	for _, s := range x["runcmd"].([]interface{}) {
		scripts = append(scripts, s.(string))
	}
	return scripts
}

type line struct {
	index int
	line  string
}

func assertScriptMatch(c *gc.C, got []string, expect string, exact bool) {
	for _, s := range got {
		c.Logf("script: %s", regexp.QuoteMeta(strings.Replace(s, "\n", "\\n", -1)))
	}
	var pats []line
	for i, pat := range strings.Split(strings.Trim(expect, "\n"), "\n") {
		pats = append(pats, line{
			index: i,
			line:  pat,
		})
	}
	var scripts []line
	for i := range got {
		scripts = append(scripts, line{
			index: i,
			line:  strings.Replace(got[i], "\n", "\\n", -1), // make .* work
		})
	}
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
				panic("unreachable")
			} else {
				scripts = scripts[1:]
			}
		}
	}
}

// CheckPackage checks that the cloudinit will or won't install the given
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
		if p == pkg {
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

// CheckAptSources checks that the cloudinit will or won't install the given
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
		cfg.StateInfo = nil
	}},
	{"missing API info", func(cfg *cloudinit.MachineConfig) {
		cfg.APIInfo = nil
	}},
	{"missing syslog port", func(cfg *cloudinit.MachineConfig) {
		cfg.SyslogPort = 0
	}},
	{"missing state hosts", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		cfg.StateInfo = &state.Info{
			Tag:    "machine-99",
			CACert: []byte(testing.CACert),
		}
		cfg.APIInfo = &api.Info{
			Addrs:  []string{"foo:35"},
			Tag:    "machine-99",
			CACert: []byte(testing.CACert),
		}
	}},
	{"missing API hosts", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		cfg.StateInfo = &state.Info{
			Addrs:  []string{"foo:35"},
			Tag:    "machine-99",
			CACert: []byte(testing.CACert),
		}
		cfg.APIInfo = &api.Info{
			Tag:    "machine-99",
			CACert: []byte(testing.CACert),
		}
	}},
	{"missing CA certificate", func(cfg *cloudinit.MachineConfig) {
		cfg.StateInfo = &state.Info{Addrs: []string{"host:98765"}}
	}},
	{"missing CA certificate", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		cfg.StateInfo = &state.Info{
			Tag:   "machine-99",
			Addrs: []string{"host:98765"},
		}
	}},
	{"missing state server certificate", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServerCert = []byte{}
	}},
	{"missing state server private key", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServerKey = []byte{}
	}},
	{"missing var directory", func(cfg *cloudinit.MachineConfig) {
		cfg.DataDir = ""
	}},
	{"missing tools", func(cfg *cloudinit.MachineConfig) {
		cfg.Tools = nil
	}},
	{"missing tools URL", func(cfg *cloudinit.MachineConfig) {
		cfg.Tools = &tools.Tools{}
	}},
	{"entity tag must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		info := *cfg.StateInfo
		info.Tag = "machine-0"
		cfg.StateInfo = &info
	}},
	{"entity tag must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		info := *cfg.StateInfo
		info.Tag = ""
		cfg.StateInfo = &info
	}},
	{"entity tag must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		info := *cfg.APIInfo
		info.Tag = "machine-0"
		cfg.APIInfo = &info
	}},
	{"entity tag must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		info := *cfg.APIInfo
		info.Tag = ""
		cfg.APIInfo = &info
	}},
	{"entity tag must be blank when starting a state server", func(cfg *cloudinit.MachineConfig) {
		info := *cfg.StateInfo
		info.Tag = "machine-0"
		cfg.StateInfo = &info
	}},
	{"entity tag must be blank when starting a state server", func(cfg *cloudinit.MachineConfig) {
		info := *cfg.APIInfo
		info.Tag = "machine-0"
		cfg.APIInfo = &info
	}},
	{"missing state port", func(cfg *cloudinit.MachineConfig) {
		cfg.StatePort = 0
	}},
	{"missing API port", func(cfg *cloudinit.MachineConfig) {
		cfg.APIPort = 0
	}},
	{"missing machine nonce", func(cfg *cloudinit.MachineConfig) {
		cfg.MachineNonce = ""
	}},
}

// TestCloudInitVerify checks that required fields are appropriately
// checked for by NewCloudInit.
func (*cloudinitSuite) TestCloudInitVerify(c *gc.C) {
	cfg := &cloudinit.MachineConfig{
		StateServer:      true,
		StateServerCert:  serverCert,
		StateServerKey:   serverKey,
		StatePort:        1234,
		APIPort:          1235,
		SyslogPort:       2345,
		MachineId:        "99",
		Tools:            newSimpleTools("9.9.9-linux-arble"),
		AuthorizedKeys:   "sshkey1",
		AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
		StateInfo: &state.Info{
			Addrs:    []string{"host:98765"},
			CACert:   []byte(testing.CACert),
			Password: "password",
		},
		APIInfo: &api.Info{
			Addrs:  []string{"host:9999"},
			CACert: []byte(testing.CACert),
		},
		Config:       minimalConfig(c),
		DataDir:      environs.DataDir,
		MachineNonce: "FAKE_NONCE",
	}
	// check that the base configuration does not give an error
	ci := coreCloudinit.New()
	err := cloudinit.Configure(cfg, ci)
	c.Assert(err, gc.IsNil)

	for i, test := range verifyTests {
		c.Logf("test %d. %s", i, test.err)
		cfg1 := *cfg
		test.mutate(&cfg1)
		err = cloudinit.Configure(&cfg1, ci)
		c.Assert(err, gc.ErrorMatches, "invalid machine configuration: "+test.err)
	}
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
