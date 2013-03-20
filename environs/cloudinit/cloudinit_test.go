package cloudinit_test

import (
	"encoding/base64"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
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

var envConfig = mustNewConfig(map[string]interface{}{
	"type":            "ec2",
	"name":            "foo",
	"default-series":  "series",
	"authorized-keys": "keys",
	"ca-cert":         testing.CACert,
})

func mustNewConfig(m map[string]interface{}) *config.Config {
	cfg, err := config.New(m)
	if err != nil {
		panic(err)
	}
	return cfg
}

type cloudinitTest struct {
	cfg           cloudinit.MachineConfig
	expectScripts string
}

// Each test gives a cloudinit config - we check the
// output to see if it looks correct.
var cloudinitTests = []cloudinitTest{{
	cfg: cloudinit.MachineConfig{
		MachineId:       "0",
		AuthorizedKeys:  "sshkey1",
		Tools:           newSimpleTools("1.2.3-linux-amd64"),
		MongoURL:        "http://juju-dist.host.com/mongodb.tar.gz",
		StateServer:     true,
		StateServerCert: serverCert,
		StateServerKey:  serverKey,
		MongoPort:       37017,
		APIPort:         17070,
		StateInfo: &state.Info{
			Password: "arble",
			CACert:   []byte("CA CERT\n" + testing.CACert),
		},
		APIInfo: &api.Info{
			Password: "bletch",
			CACert:   []byte("CA CERT\n" + testing.CACert),
		},
		Config:  envConfig,
		DataDir: "/var/lib/juju",
	},
	expectScripts: `
mkdir -p /var/lib/juju
mkdir -p /var/log/juju
bin='/var/lib/juju/tools/1\.2\.3-linux-amd64'
mkdir -p \$bin
wget --no-verbose -O - 'http://foo\.com/tools/juju1\.2\.3-linux-amd64\.tgz' \| tar xz -C \$bin
echo -n 'http://foo\.com/tools/juju1\.2\.3-linux-amd64\.tgz' > \$bin/downloaded-url\.txt
echo 'SERVER CERT\\n[^']*SERVER KEY\\n[^']*' > '/var/lib/juju/server\.pem'
chmod 600 '/var/lib/juju/server\.pem'
mkdir -p /opt
wget --no-verbose -O - 'http://juju-dist\.host\.com/mongodb\.tar\.gz' \| tar xz -C /opt
mkdir -p /var/lib/juju/db/journal
dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc\.0
dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc\.1
dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc\.2
cat >> /etc/init/juju-db\.conf << 'EOF'\\ndescription "juju state database"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\n\\nexec /opt/mongo/bin/mongod --auth --dbpath=/var/lib/juju/db --sslOnNormalPorts --sslPEMKeyFile '/var/lib/juju/server\.pem' --sslPEMKeyPassword ignored --bind_ip 0\.0\.0\.0 --port 37017 --noprealloc --smallfiles\\nEOF\\n
start juju-db
mkdir -p '/var/lib/juju/agents/bootstrap'
echo 'datadir: /var/lib/juju\\nstateservercert:\\n[^']+stateserverkey:\\n[^']+mongoport: 37017\\napiport: 17070\\noldpassword: arble\\nstateinfo:\\n  addrs:\\n  - localhost:37017\\n  cacert:\\n[^']+  entityname: bootstrap\\n  password: ""\\noldapipassword: ""\\napiinfo:\\n  addrs:\\n  - localhost:17070\\n  cacert:\\n[^']+  entityname: bootstrap\\n  password: ""\\n' > '/var/lib/juju/agents/bootstrap/agent\.conf'
chmod 600 '/var/lib/juju/agents/bootstrap/agent\.conf'
/var/lib/juju/tools/1\.2\.3-linux-amd64/jujud bootstrap-state --data-dir '/var/lib/juju' --env-config '[^']*' --debug
rm -rf '/var/lib/juju/agents/bootstrap'
rm -f /etc/rsyslog.d/25-juju.conf
echo '' >> /etc/rsyslog.d/25-juju.conf
echo '\$ModLoad imfile' >> /etc/rsyslog.d/25-juju.conf
echo '' >> /etc/rsyslog.d/25-juju.conf
echo '\$InputFilePollInterval 5' >> /etc/rsyslog.d/25-juju.conf
echo '\$InputFileName /var/log/juju/machine-0.log' >> /etc/rsyslog.d/25-juju.conf
echo '\$InputFileTag local-juju-machine-0:' >> /etc/rsyslog.d/25-juju.conf
echo '\$InputFileStateFile machine-0' >> /etc/rsyslog.d/25-juju.conf
echo '\$InputRunFileMonitor' >> /etc/rsyslog.d/25-juju.conf
echo '' >> /etc/rsyslog.d/25-juju.conf
echo '\$ModLoad imudp' >> /etc/rsyslog.d/25-juju.conf
echo '\$UDPServerRun 514' >> /etc/rsyslog.d/25-juju.conf
echo '' >> /etc/rsyslog.d/25-juju.conf
echo '# Messages received from remote rsyslog machines contain a leading space so we' >> /etc/rsyslog.d/25-juju.conf
echo '# need to account for that.' >> /etc/rsyslog.d/25-juju.conf
echo '\$template JujuLogFormatLocal,"%HOSTNAME%:%msg:::drop-last-lf%\\\\n"' >> /etc/rsyslog.d/25-juju.conf
echo '\$template JujuLogFormat,\"%HOSTNAME%:%msg:1:2048:drop-last-lf%\\\\n\"' >> /etc/rsyslog.d/25-juju.conf
echo '' >> /etc/rsyslog.d/25-juju.conf
echo ':syslogtag, startswith, "juju-" /var/log/juju/all-machines.log;JujuLogFormat' >> /etc/rsyslog.d/25-juju.conf
echo ':syslogtag, startswith, "local-juju-" /var/log/juju/all-machines.log;JujuLogFormatLocal' >> /etc/rsyslog.d/25-juju.conf
echo '& ~' >> /etc/rsyslog.d/25-juju.conf
echo '' >> /etc/rsyslog.d/25-juju.conf
restart rsyslog
mkdir -p '/var/lib/juju/agents/machine-0'
echo 'datadir: /var/lib/juju\\nstateservercert:\\n[^']+stateserverkey:\\n[^']+mongoport: 37017\\napiport: 17070\\noldpassword: arble\\nstateinfo:\\n  addrs:\\n  - localhost:37017\\n  cacert:\\n[^']+  entityname: machine-0\\n  password: ""\\noldapipassword: ""\\napiinfo:\\n  addrs:\\n  - localhost:17070\\n  cacert:\\n[^']+  entityname: machine-0\\n  password: ""\\n' > '/var/lib/juju/agents/machine-0/agent\.conf'
chmod 600 '/var/lib/juju/agents/machine-0/agent\.conf'
ln -s 1\.2\.3-linux-amd64 '/var/lib/juju/tools/machine-0'
cat >> /etc/init/jujud-machine-0\.conf << 'EOF'\\ndescription "juju machine-0 agent"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\n\\nexec /var/lib/juju/tools/machine-0/jujud machine --log-file /var/log/juju/machine-0\.log --data-dir '/var/lib/juju' --machine-id 0  --debug >> /var/log/juju/machine-0\.log 2>&1\\nEOF\\n
start jujud-machine-0
`,
},
	{
		cfg: cloudinit.MachineConfig{
			MachineId:      "99",
			AuthorizedKeys: "sshkey1",
			DataDir:        "/var/lib/juju",
			StateServer:    false,
			Tools:          newSimpleTools("1.2.3-linux-amd64"),
			StateInfo: &state.Info{
				Addrs:      []string{"state-addr.example.com:12345"},
				EntityName: "machine-99",
				Password:   "arble",
				CACert:     []byte("CA CERT\n" + testing.CACert),
			},
			APIInfo: &api.Info{
				Addrs:      []string{"state-addr.example.com:54321"},
				EntityName: "machine-99",
				Password:   "bletch",
				CACert:     []byte("CA CERT\n" + testing.CACert),
			},
		},
		expectScripts: `
mkdir -p /var/lib/juju
mkdir -p /var/log/juju
bin='/var/lib/juju/tools/1\.2\.3-linux-amd64'
mkdir -p \$bin
wget --no-verbose -O - 'http://foo\.com/tools/juju1\.2\.3-linux-amd64\.tgz' \| tar xz -C \$bin
echo -n 'http://foo\.com/tools/juju1\.2\.3-linux-amd64\.tgz' > \$bin/downloaded-url\.txt
rm -f /etc/rsyslog.d/25-juju.conf
echo '' >> /etc/rsyslog.d/25-juju.conf
echo '\$ModLoad imfile' >> /etc/rsyslog.d/25-juju.conf
echo '' >> /etc/rsyslog.d/25-juju.conf
echo '\$InputFilePollInterval 5' >> /etc/rsyslog.d/25-juju.conf
echo '\$InputFileName /var/log/juju/machine-99.log' >> /etc/rsyslog.d/25-juju.conf
echo '\$InputFileTag juju-machine-99:' >> /etc/rsyslog.d/25-juju.conf
echo '\$InputFileStateFile machine-99' >> /etc/rsyslog.d/25-juju.conf
echo '\$InputRunFileMonitor' >> /etc/rsyslog.d/25-juju.conf
echo '' >> /etc/rsyslog.d/25-juju.conf
echo ':syslogtag, startswith, "juju-" @state-addr.example.com:514' >> /etc/rsyslog.d/25-juju.conf
echo '& ~' >> /etc/rsyslog.d/25-juju.conf
echo '' >> /etc/rsyslog.d/25-juju.conf
restart rsyslog
mkdir -p '/var/lib/juju/agents/machine-99'
echo 'datadir: /var/lib/juju\\noldpassword: arble\\nstateinfo:\\n  addrs:\\n  - state-addr\.example\.com:12345\\n  cacert:\\n[^']+  entityname: machine-99\\n  password: ""\\noldapipassword: ""\\napiinfo:\\n  addrs:\\n  - state-addr\.example\.com:54321\\n  cacert:\\n[^']+  entityname: machine-99\\n  password: ""\\n' > '/var/lib/juju/agents/machine-99/agent\.conf'
chmod 600 '/var/lib/juju/agents/machine-99/agent\.conf'
ln -s 1\.2\.3-linux-amd64 '/var/lib/juju/tools/machine-99'
cat >> /etc/init/jujud-machine-99\.conf << 'EOF'\\ndescription "juju machine-99 agent"\\nauthor "Juju Team <juju@lists\.ubuntu\.com>"\\nstart on runlevel \[2345\]\\nstop on runlevel \[!2345\]\\nrespawn\\nnormal exit 0\\n\\nexec /var/lib/juju/tools/machine-99/jujud machine --log-file /var/log/juju/machine-99\.log --data-dir '/var/lib/juju' --machine-id 99  --debug >> /var/log/juju/machine-99\.log 2>&1\\nEOF\\n
start jujud-machine-99
`,
	},
}

func newSimpleTools(vers string) *state.Tools {
	return &state.Tools{
		URL:    "http://foo.com/tools/juju" + vers + ".tgz",
		Binary: version.MustParseBinary(vers),
	}
}

func (t *cloudinitTest) check(c *C) {
	ci, err := cloudinit.New(&t.cfg)
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
	scriptDiff(c, scripts, t.expectScripts)
	if t.cfg.Config != nil {
		checkEnvConfig(c, t.cfg.Config, x, scripts)
	}
	checkPackage(c, x, "git", true)
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
		ci, err := cloudinit.New(&test.cfg)
		c.Assert(err, IsNil)
		c.Check(ci, NotNil)

		test.check(c)
	}
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
			EntityName: "machine-99",
			CACert:     []byte(testing.CACert),
		}
		cfg.APIInfo = &api.Info{
			Addrs:      []string{"foo:35"},
			EntityName: "machine-99",
			CACert:     []byte(testing.CACert),
		}
	}},
	{"missing API hosts", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		cfg.StateInfo = &state.Info{
			Addrs:      []string{"foo:35"},
			EntityName: "machine-99",
			CACert:     []byte(testing.CACert),
		}
		cfg.APIInfo = &api.Info{
			EntityName: "machine-99",
			CACert:     []byte(testing.CACert),
		}
	}},
	{"missing CA certificate", func(cfg *cloudinit.MachineConfig) {
		cfg.StateInfo = &state.Info{Addrs: []string{"host:98765"}}
	}},
	{"missing CA certificate", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		cfg.StateInfo = &state.Info{
			EntityName: "machine-99",
			Addrs:      []string{"host:98765"},
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
		cfg.Tools = &state.Tools{}
	}},
	{"entity name must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		info := *cfg.StateInfo
		info.EntityName = "machine-0"
		cfg.StateInfo = &info
	}},
	{"entity name must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		info := *cfg.StateInfo
		info.EntityName = ""
		cfg.StateInfo = &info
	}},
	{"entity name must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		info := *cfg.APIInfo
		info.EntityName = "machine-0"
		cfg.APIInfo = &info
	}},
	{"entity name must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		info := *cfg.APIInfo
		info.EntityName = ""
		cfg.APIInfo = &info
	}},
	{"entity name must be blank when starting a state server", func(cfg *cloudinit.MachineConfig) {
		info := *cfg.StateInfo
		info.EntityName = "machine-0"
		cfg.StateInfo = &info
	}},
	{"entity name must be blank when starting a state server", func(cfg *cloudinit.MachineConfig) {
		info := *cfg.APIInfo
		info.EntityName = "machine-0"
		cfg.APIInfo = &info
	}},
	{"missing mongo port", func(cfg *cloudinit.MachineConfig) {
		cfg.MongoPort = 0
	}},
	{"missing API port", func(cfg *cloudinit.MachineConfig) {
		cfg.APIPort = 0
	}},
}

// TestCloudInitVerify checks that required fields are appropriately
// checked for by NewCloudInit.
func (*cloudinitSuite) TestCloudInitVerify(c *C) {
	cfg := &cloudinit.MachineConfig{
		StateServer:     true,
		StateServerCert: serverCert,
		StateServerKey:  serverKey,
		MongoPort:       1234,
		APIPort:         1235,
		MachineId:       "99",
		Tools:           newSimpleTools("9.9.9-linux-arble"),
		AuthorizedKeys:  "sshkey1",
		StateInfo: &state.Info{
			Addrs:  []string{"host:98765"},
			CACert: []byte(testing.CACert),
		},
		APIInfo: &api.Info{
			Addrs:  []string{"host:9999"},
			CACert: []byte(testing.CACert),
		},
		Config:  envConfig,
		DataDir: "/var/lib/juju",
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
