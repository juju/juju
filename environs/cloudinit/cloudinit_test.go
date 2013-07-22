// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"encoding/base64"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"regexp"
	"strings"
)

// Use local suite since this file lives in the ec2 package
// for testing internals.
type cloudinitSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&cloudinitSuite{})

var envConstraints = constraints.MustParse("mem=2G")

type cloudinitTest struct {
	cfg           cloudinit.MachineConfig
	setEnvConfig  bool
	expectScripts string
}

func minimalConfig(c *C) *config.Config {
	cfg, err := config.New(map[string]interface{}{
		"type":            "test",
		"name":            "test-name",
		"default-series":  "test-series",
		"authorized-keys": "test-keys",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, IsNil)
	return cfg
}

// Each test gives a cloudinit config - we check the
// output to see if it looks correct.
var cloudinitTests = []cloudinitTest{
	{
		// precise state server
		cfg: cloudinit.MachineConfig{
			MachineId:      "0",
			AuthorizedKeys: "sshkey1",
			ProviderType:   "dummy",
			// precise currently needs mongo from PPA
			Tools:           newSimpleTools("1.2.3-precise-amd64"),
			StateServer:     true,
			StateServerCert: serverCert,
			StateServerKey:  serverKey,
			StatePort:       37017,
			APIPort:         17070,
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
set -xe
mkdir -p /var/lib/juju
mkdir -p /var/log/juju
bin='/var/lib/juju/tools/1\.2\.3-precise-amd64'
mkdir -p \$bin
wget --no-verbose -O - 'http://foo\.com/tools/juju1\.2\.3-precise-amd64\.tgz' \| tar xz -C \$bin
echo -n 'http://foo\.com/tools/juju1\.2\.3-precise-amd64\.tgz' > \$bin/downloaded-url\.txt
cat > /etc/rsyslog.d/25-juju.conf << 'EOF'\\n\\n\$ModLoad imfile\\n\\n\$InputFileStateFile /var/spool/rsyslog/juju-machine-0-state\\n\$InputFilePersistStateInterval 50\\n\$InputFilePollInterval 5\\n\$InputFileName /var/log/juju/machine-0.log\\n\$InputFileTag local-juju-machine-0:\\n\$InputFileStateFile machine-0\\n\$InputRunFileMonitor\\n\\n\$ModLoad imudp\\n\$UDPServerRun 514\\n\\n# Messages received from remote rsyslog machines contain a leading space so we\\n# need to account for that.\\n\$template JujuLogFormatLocal,\"%HOSTNAME%:%msg:::drop-last-lf%\\n\"\\n\$template JujuLogFormat,\"%HOSTNAME%:%msg:2:2048:drop-last-lf%\\n\"\\n\\n:syslogtag, startswith, \"juju-\" /var/log/juju/all-machines.log;JujuLogFormat\\n:syslogtag, startswith, \"local-juju-\" /var/log/juju/all-machines.log;JujuLogFormatLocal\\n& ~\\nEOF\\n
restart rsyslog
mkdir -p '/var/lib/juju/agents/machine-0'
echo 'datadir: /var/lib/juju\\nstateservercert:\\n[^']+stateserverkey:\\n[^']+stateport: 37017\\napiport: 17070\\noldpassword: arble\\nmachinenonce: FAKE_NONCE\\nstateinfo:\\n  addrs:\\n  - localhost:37017\\n  cacert:\\n[^']+  tag: machine-0\\n  password: ""\\noldapipassword: ""\\napiinfo:\\n  addrs:\\n  - localhost:17070\\n  cacert:\\n[^']+  tag: machine-0\\n  password: ""\\n' > '/var/lib/juju/agents/machine-0/agent\.conf'
chmod 600 '/var/lib/juju/agents/machine-0/agent\.conf'
echo 'SERVER CERT\\n[^']*SERVER KEY\\n[^']*' > '/var/lib/juju/server\.pem'
chmod 600 '/var/lib/juju/server\.pem'
mkdir -p /var/lib/juju/db/journal
dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc\.0
dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc\.1
dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc\.2
cat >> /etc/init/juju-db\.conf << 'EOF'\\ndescription "juju state database"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\n\\nlimit nofile 65000 65000\\nlimit nproc 20000 20000\\n\\nexec /usr/bin/mongod --auth --dbpath=/var/lib/juju/db --sslOnNormalPorts --sslPEMKeyFile '/var/lib/juju/server\.pem' --sslPEMKeyPassword ignored --bind_ip 0\.0\.0\.0 --port 37017 --noprealloc --syslog --smallfiles\\nEOF\\n
start juju-db
mkdir -p '/var/lib/juju/agents/bootstrap'
echo 'datadir: /var/lib/juju\\nstateservercert:\\n[^']+stateserverkey:\\n[^']+stateport: 37017\\napiport: 17070\\noldpassword: arble\\nmachinenonce: FAKE_NONCE\\nstateinfo:\\n  addrs:\\n  - localhost:37017\\n  cacert:\\n[^']+  tag: bootstrap\\n  password: ""\\noldapipassword: ""\\napiinfo:\\n  addrs:\\n  - localhost:17070\\n  cacert:\\n[^']+  tag: bootstrap\\n  password: ""\\n' > '/var/lib/juju/agents/bootstrap/agent\.conf'
chmod 600 '/var/lib/juju/agents/bootstrap/agent\.conf'
echo 'some-url' > /tmp/provider-state-url
/var/lib/juju/tools/1\.2\.3-precise-amd64/jujud bootstrap-state --data-dir '/var/lib/juju' --env-config '[^']*' --constraints 'mem=2048M' --debug
rm -rf '/var/lib/juju/agents/bootstrap'
ln -s 1\.2\.3-precise-amd64 '/var/lib/juju/tools/machine-0'
cat >> /etc/init/jujud-machine-0\.conf << 'EOF'\\ndescription "juju machine-0 agent"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\nenv JUJU_PROVIDER_TYPE="dummy"\\n\\nlimit nofile 20000 20000\\n\\nexec /var/lib/juju/tools/machine-0/jujud machine --log-file '/var/log/juju/machine-0\.log' --data-dir '/var/lib/juju' --machine-id 0  --debug >> /var/log/juju/machine-0\.log 2>&1\\nEOF\\n
start jujud-machine-0
`,
	}, {
		// raring state server
		cfg: cloudinit.MachineConfig{
			MachineId:      "0",
			AuthorizedKeys: "sshkey1",
			ProviderType:   "dummy",
			// raring provides mongo in the archive
			Tools:           newSimpleTools("1.2.3-raring-amd64"),
			StateServer:     true,
			StateServerCert: serverCert,
			StateServerKey:  serverKey,
			StatePort:       37017,
			APIPort:         17070,
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
set -xe
mkdir -p /var/lib/juju
mkdir -p /var/log/juju
bin='/var/lib/juju/tools/1\.2\.3-raring-amd64'
mkdir -p \$bin
wget --no-verbose -O - 'http://foo\.com/tools/juju1\.2\.3-raring-amd64\.tgz' \| tar xz -C \$bin
echo -n 'http://foo\.com/tools/juju1\.2\.3-raring-amd64\.tgz' > \$bin/downloaded-url\.txt
cat > /etc/rsyslog.d/25-juju.conf << 'EOF'\\n\\n\$ModLoad imfile\\n\\n\$InputFileStateFile /var/spool/rsyslog/juju-machine-0-state\\n\$InputFilePersistStateInterval 50\\n\$InputFilePollInterval 5\\n\$InputFileName /var/log/juju/machine-0.log\\n\$InputFileTag local-juju-machine-0:\\n\$InputFileStateFile machine-0\\n\$InputRunFileMonitor\\n\\n\$ModLoad imudp\\n\$UDPServerRun 514\\n\\n# Messages received from remote rsyslog machines contain a leading space so we\\n# need to account for that.\\n\$template JujuLogFormatLocal,\"%HOSTNAME%:%msg:::drop-last-lf%\\n\"\\n\$template JujuLogFormat,\"%HOSTNAME%:%msg:2:2048:drop-last-lf%\\n\"\\n\\n:syslogtag, startswith, \"juju-\" /var/log/juju/all-machines.log;JujuLogFormat\\n:syslogtag, startswith, \"local-juju-\" /var/log/juju/all-machines.log;JujuLogFormatLocal\\n& ~\\nEOF\\n
restart rsyslog
mkdir -p '/var/lib/juju/agents/machine-0'
echo 'datadir: /var/lib/juju\\nstateservercert:\\n[^']+stateserverkey:\\n[^']+stateport: 37017\\napiport: 17070\\noldpassword: arble\\nmachinenonce: FAKE_NONCE\\nstateinfo:\\n  addrs:\\n  - localhost:37017\\n  cacert:\\n[^']+  tag: machine-0\\n  password: ""\\noldapipassword: ""\\napiinfo:\\n  addrs:\\n  - localhost:17070\\n  cacert:\\n[^']+  tag: machine-0\\n  password: ""\\n' > '/var/lib/juju/agents/machine-0/agent\.conf'
chmod 600 '/var/lib/juju/agents/machine-0/agent\.conf'
echo 'SERVER CERT\\n[^']*SERVER KEY\\n[^']*' > '/var/lib/juju/server\.pem'
chmod 600 '/var/lib/juju/server\.pem'
mkdir -p /var/lib/juju/db/journal
dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc\.0
dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc\.1
dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc\.2
cat >> /etc/init/juju-db\.conf << 'EOF'\\ndescription "juju state database"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\n\\nlimit nofile 65000 65000\\nlimit nproc 20000 20000\\n\\nexec /usr/bin/mongod --auth --dbpath=/var/lib/juju/db --sslOnNormalPorts --sslPEMKeyFile '/var/lib/juju/server\.pem' --sslPEMKeyPassword ignored --bind_ip 0\.0\.0\.0 --port 37017 --noprealloc --syslog --smallfiles\\nEOF\\n
start juju-db
mkdir -p '/var/lib/juju/agents/bootstrap'
echo 'datadir: /var/lib/juju\\nstateservercert:\\n[^']+stateserverkey:\\n[^']+stateport: 37017\\napiport: 17070\\noldpassword: arble\\nmachinenonce: FAKE_NONCE\\nstateinfo:\\n  addrs:\\n  - localhost:37017\\n  cacert:\\n[^']+  tag: bootstrap\\n  password: ""\\noldapipassword: ""\\napiinfo:\\n  addrs:\\n  - localhost:17070\\n  cacert:\\n[^']+  tag: bootstrap\\n  password: ""\\n' > '/var/lib/juju/agents/bootstrap/agent\.conf'
chmod 600 '/var/lib/juju/agents/bootstrap/agent\.conf'
echo 'some-url' > /tmp/provider-state-url
/var/lib/juju/tools/1\.2\.3-raring-amd64/jujud bootstrap-state --data-dir '/var/lib/juju' --env-config '[^']*' --constraints 'mem=2048M' --debug
rm -rf '/var/lib/juju/agents/bootstrap'
ln -s 1\.2\.3-raring-amd64 '/var/lib/juju/tools/machine-0'
cat >> /etc/init/jujud-machine-0\.conf << 'EOF'\\ndescription "juju machine-0 agent"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\nenv JUJU_PROVIDER_TYPE="dummy"\\n\\nlimit nofile 20000 20000\\n\\nexec /var/lib/juju/tools/machine-0/jujud machine --log-file '/var/log/juju/machine-0\.log' --data-dir '/var/lib/juju' --machine-id 0  --debug >> /var/log/juju/machine-0\.log 2>&1\\nEOF\\n
start jujud-machine-0
`,
	}, {
		cfg: cloudinit.MachineConfig{
			MachineId:      "99",
			AuthorizedKeys: "sshkey1",
			ProviderType:   "dummy",
			DataDir:        environs.DataDir,
			StateServer:    false,
			Tools:          newSimpleTools("1.2.3-linux-amd64"),
			MachineNonce:   "FAKE_NONCE",
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
		},
		expectScripts: `
set -xe
mkdir -p /var/lib/juju
mkdir -p /var/log/juju
bin='/var/lib/juju/tools/1\.2\.3-linux-amd64'
mkdir -p \$bin
wget --no-verbose -O - 'http://foo\.com/tools/juju1\.2\.3-linux-amd64\.tgz' \| tar xz -C \$bin
echo -n 'http://foo\.com/tools/juju1\.2\.3-linux-amd64\.tgz' > \$bin/downloaded-url\.txt
cat > /etc/rsyslog.d/25-juju.conf << 'EOF'\\n\\n\$ModLoad imfile\\n\\n\$InputFileStateFile /var/spool/rsyslog/juju-machine-99-state\\n\$InputFilePersistStateInterval 50\\n\$InputFilePollInterval 5\\n\$InputFileName /var/log/juju/machine-99.log\\n\$InputFileTag juju-machine-99:\\n\$InputFileStateFile machine-99\\n\$InputRunFileMonitor\\n\\n:syslogtag, startswith, \"juju-\" @state-addr.testing.invalid:514\\n& ~\\nEOF\\n
restart rsyslog
mkdir -p '/var/lib/juju/agents/machine-99'
echo 'datadir: /var/lib/juju\\noldpassword: arble\\nmachinenonce: FAKE_NONCE\\nstateinfo:\\n  addrs:\\n  - state-addr\.testing\.invalid:12345\\n  cacert:\\n[^']+  tag: machine-99\\n  password: ""\\noldapipassword: ""\\napiinfo:\\n  addrs:\\n  - state-addr\.testing\.invalid:54321\\n  cacert:\\n[^']+  tag: machine-99\\n  password: ""\\n' > '/var/lib/juju/agents/machine-99/agent\.conf'
chmod 600 '/var/lib/juju/agents/machine-99/agent\.conf'
ln -s 1\.2\.3-linux-amd64 '/var/lib/juju/tools/machine-99'
cat >> /etc/init/jujud-machine-99\.conf << 'EOF'\\ndescription "juju machine-99 agent"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\nenv JUJU_PROVIDER_TYPE="dummy"\\n\\nlimit nofile 20000 20000\\n\\nexec /var/lib/juju/tools/machine-99/jujud machine --log-file '/var/log/juju/machine-99\.log' --data-dir '/var/lib/juju' --machine-id 99  --debug >> /var/log/juju/machine-99\.log 2>&1\\nEOF\\n
start jujud-machine-99
`,
	}, {
		cfg: cloudinit.MachineConfig{
			MachineId:            "2/lxc/1",
			MachineContainerType: "lxc",
			AuthorizedKeys:       "sshkey1",
			ProviderType:         "dummy",
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
		},
		expectScripts: `
set -xe
mkdir -p /var/lib/juju
mkdir -p /var/log/juju
bin='/var/lib/juju/tools/1\.2\.3-linux-amd64'
mkdir -p \$bin
wget --no-verbose -O - 'http://foo\.com/tools/juju1\.2\.3-linux-amd64\.tgz' \| tar xz -C \$bin
echo -n 'http://foo\.com/tools/juju1\.2\.3-linux-amd64\.tgz' > \$bin/downloaded-url\.txt
cat > /etc/rsyslog.d/25-juju.conf << 'EOF'\\n\\n\$ModLoad imfile\\n\\n\$InputFileStateFile /var/spool/rsyslog/juju-machine-2-lxc-1-state\\n\$InputFilePersistStateInterval 50\\n\$InputFilePollInterval 5\\n\$InputFileName /var/log/juju/machine-2-lxc-1.log\\n\$InputFileTag juju-machine-2-lxc-1:\\n\$InputFileStateFile machine-2-lxc-1\\n\$InputRunFileMonitor\\n\\n:syslogtag, startswith, \"juju-\" @state-addr.testing.invalid:514\\n& ~\\nEOF\\n
restart rsyslog
mkdir -p '/var/lib/juju/agents/machine-2-lxc-1'
echo 'datadir: /var/lib/juju\\noldpassword: arble\\nmachinenonce: FAKE_NONCE\\nstateinfo:\\n  addrs:\\n  - state-addr\.testing\.invalid:12345\\n  cacert:\\n[^']+  tag: machine-2-lxc-1\\n  password: ""\\noldapipassword: ""\\napiinfo:\\n  addrs:\\n  - state-addr\.testing\.invalid:54321\\n  cacert:\\n[^']+  tag: machine-2-lxc-1\\n  password: ""\\n' > '/var/lib/juju/agents/machine-2-lxc-1/agent\.conf'
chmod 600 '/var/lib/juju/agents/machine-2-lxc-1/agent\.conf'
ln -s 1\.2\.3-linux-amd64 '/var/lib/juju/tools/machine-2-lxc-1'
cat >> /etc/init/jujud-machine-2-lxc-1\.conf << 'EOF'\\ndescription "juju machine-2-lxc-1 agent"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\nenv JUJU_PROVIDER_TYPE="dummy"\\n\\nlimit nofile 20000 20000\\n\\nexec /var/lib/juju/tools/machine-2-lxc-1/jujud machine --log-file '/var/log/juju/machine-2-lxc-1\.log' --data-dir '/var/lib/juju' --machine-id 2/lxc/1  --debug >> /var/log/juju/machine-2-lxc-1\.log 2>&1\\nEOF\\n
start jujud-machine-2-lxc-1
`,
	},
}

func newSimpleTools(vers string) *tools.Tools {
	return &tools.Tools{
		URL:    "http://foo.com/tools/juju" + vers + ".tgz",
		Binary: version.MustParseBinary(vers),
	}
}

// check that any --env-config $base64 is valid and matches t.cfg.Config
func checkEnvConfig(c *C, cfg *config.Config, x map[interface{}]interface{}, scripts []string) {
	re := regexp.MustCompile(`--env-config '([\w,=]+)'`)
	found := false
	for _, s := range scripts {
		m := re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		found = true
		buf, err := base64.StdEncoding.DecodeString(m[1])
		c.Assert(err, IsNil)
		var actual map[string]interface{}
		err = goyaml.Unmarshal(buf, &actual)
		c.Assert(err, IsNil)
		c.Assert(cfg.AllAttrs(), DeepEquals, actual)
	}
	c.Assert(found, Equals, true)
}

// TestCloudInit checks that the output from the various tests
// in cloudinitTests is well formed.
func (*cloudinitSuite) TestCloudInit(c *C) {
	for i, test := range cloudinitTests {
		c.Logf("test %d", i)
		if test.setEnvConfig {
			test.cfg.Config = minimalConfig(c)
		}
		ci, err := cloudinit.New(&test.cfg)
		c.Assert(err, IsNil)
		c.Check(ci, NotNil)
		// render the cloudinit config to bytes, and then
		// back to a map so we can introspect it without
		// worrying about internal details of the cloudinit
		// package.
		data, err := ci.Render()
		c.Assert(err, IsNil)

		x := make(map[interface{}]interface{})
		err = goyaml.Unmarshal(data, &x)
		c.Assert(err, IsNil)

		c.Check(x["apt_upgrade"], Equals, true)
		c.Check(x["apt_update"], Equals, true)

		scripts := getScripts(x)
		scriptDiff(c, scripts, test.expectScripts)
		if test.cfg.Config != nil {
			checkEnvConfig(c, test.cfg.Config, x, scripts)
		}
		checkPackage(c, x, "git", true)
		// The lxc package should only be there if the machine container type is not lxc.
		hasLxc := test.cfg.MachineContainerType != "lxc"
		checkPackage(c, x, "lxc", hasLxc)
		if test.cfg.StateServer {
			checkPackage(c, x, "mongodb-server", true)
			source := struct{ source, key string }{
				source: "ppa:juju/experimental",
				key:    "1024R/C8068B11",
			}
			checkAptSource(c, x, source, test.cfg.NeedMongoPPA())
		}
	}
}

func (*cloudinitSuite) TestCloudInitConfigure(c *C) {
	for i, test := range cloudinitTests {
		test.cfg.Config = minimalConfig(c)
		c.Logf("test %d (Configure)", i)
		cloudcfg := coreCloudinit.New()
		ci, err := cloudinit.Configure(&test.cfg, cloudcfg)
		c.Assert(err, IsNil)
		c.Check(ci, NotNil)
	}
}

func (*cloudinitSuite) TestCloudInitConfigureUsesGivenConfig(c *C) {
	// Create a simple cloudinit config with a 'runcmd' statement.
	cloudcfg := coreCloudinit.New()
	script := "test script"
	cloudcfg.AddRunCmd(script)
	cloudinitTests[0].cfg.Config = minimalConfig(c)
	ci, err := cloudinit.Configure(&cloudinitTests[0].cfg, cloudcfg)
	c.Assert(err, IsNil)
	c.Check(ci, NotNil)
	data, err := ci.Render()
	c.Assert(err, IsNil)

	ciContent := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, &ciContent)
	c.Assert(err, IsNil)
	// The 'runcmd' statement is at the beginning of the list
	// of 'runcmd' statements.
	runCmd := ciContent["runcmd"].([]interface{})
	c.Check(runCmd[0], Equals, script)
}

func getScripts(x map[interface{}]interface{}) []string {
	var scripts []string
	for _, s := range x["runcmd"].([]interface{}) {
		scripts = append(scripts, s.(string))
	}
	return scripts
}

func scriptDiff(c *C, got []string, expect string) {
	for _, s := range got {
		c.Logf("script: %s", regexp.QuoteMeta(strings.Replace(s, "\n", "\\n", -1)))
	}
	pats := strings.Split(strings.Trim(expect, "\n"), "\n")
	for i := 0; ; i++ {
		switch {
		case i == len(got) && i == len(pats):
			return
		case i == len(got):
			c.Fatalf("too few scripts found (expected %q at line %d)", pats[i], i)
		case i == len(pats):
			c.Fatalf("too many scripts found (got %q at line %d)", got[i], i)
		}
		script := strings.Replace(got[i], "\n", "\\n", -1) // make .* work
		c.Assert(script, Matches, pats[i], Commentf("line %d", i))
	}
}

// CheckPackage checks that the cloudinit will or won't install the given
// package, depending on the value of match.
func checkPackage(c *C, x map[interface{}]interface{}, pkg string, match bool) {
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
func checkAptSource(c *C, x map[interface{}]interface{}, source struct{ source, key string }, match bool) {
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
		if s["source"] == source.source && s["key"] == source.key {
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
func (*cloudinitSuite) TestCloudInitVerify(c *C) {
	cfg := &cloudinit.MachineConfig{
		StateServer:     true,
		StateServerCert: serverCert,
		StateServerKey:  serverKey,
		StatePort:       1234,
		APIPort:         1235,
		MachineId:       "99",
		Tools:           newSimpleTools("9.9.9-linux-arble"),
		AuthorizedKeys:  "sshkey1",
		ProviderType:    "dummy",
		StateInfo: &state.Info{
			Addrs:  []string{"host:98765"},
			CACert: []byte(testing.CACert),
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
	_, err := cloudinit.New(cfg)
	c.Assert(err, IsNil)

	for i, test := range verifyTests {
		c.Logf("test %d. %s", i, test.err)
		cfg1 := *cfg
		test.mutate(&cfg1)
		t, err := cloudinit.New(&cfg1)
		c.Assert(err, ErrorMatches, "invalid machine configuration: "+test.err)
		c.Assert(t, IsNil)
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
