package ec2

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goamz/aws"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"os"
	"path/filepath"
	"strings"
)

// Use local suite since this file lives in the ec2 package
// for testing internals.
type configSuite struct {
	home, accessKey, secretKey string
}

var _ = Suite(configSuite{})

var configTestRegion = aws.Region{
	EC2Endpoint: "testregion.nowhere:1234",
}

var testAuth = aws.Auth{"gopher", "long teeth"}

// the mandatory fields in config.
var baseConfig = "control-bucket: x\n"

// the result of parsing baseConfig.
var baseConfigResult = providerConfig{
	name:           "testenv",
	region:         "us-east-1",
	bucket:         "x",
	auth:           testAuth,
	authorizedKeys: "sshkey\n",
}

// configTest specifies a config parsing test, checking that env when
// parsed as the ec2 section of a config file matches baseConfigResult
// when mutated by the mutate function, or that the parse matches the
// given error.
type configTest struct {
	env    string
	mutate func(*providerConfig)
	err    string
}

func (t configTest) check(c *C) {
	envs, err := environs.ReadEnvironsBytes(makeEnv(t.env))
	if t.err != "" {
		c.Check(err, ErrorMatches, t.err)
		return
	}
	c.Check(err, IsNil)
	e, err := envs.Open("testenv")
	c.Assert(err, IsNil)
	c.Assert(e, NotNil)
	c.Assert(e, FitsTypeOf, (*environ)(nil))
	tconfig := baseConfigResult
	t.mutate(&tconfig)
	c.Check(e.(*environ).config(), DeepEquals, &tconfig)
	c.Check(e.(*environ).name, Equals, tconfig.name)
}

var configTests = []configTest{
	{
		baseConfig,
		func(cfg *providerConfig) {},
		"",
	},
	{
		"region: eu-west-1\n" + baseConfig,
		func(cfg *providerConfig) {
			cfg.region = "eu-west-1"
		},
		"",
	},
	{
		"region: unknown\n" + baseConfig,
		nil,
		".*invalid region name.*",
	},
	{
		"region: configtest\n" + baseConfig,
		func(cfg *providerConfig) {
			cfg.region = "configtest"
		},
		"",
	},
	{
		"region: 666\n" + baseConfig,
		nil,
		".*expected string, got 666",
	},
	{
		"access-key: 666\n" + baseConfig,
		nil,
		".*expected string, got 666",
	},
	{
		"secret-key: 666\n" + baseConfig,
		nil,
		".*expected string, got 666",
	},
	{
		"control-bucket: 666\n",
		nil,
		".*expected string, got 666",
	},
	{
		"public-bucket: 666\n" + baseConfig,
		nil,
		".*expected string, got 666",
	},
	{
		"public-bucket: foo\n" + baseConfig,
		func(cfg *providerConfig) {
			cfg.publicBucket = "foo"
		},
		"",
	},
	{
		"access-key: jujuer\nsecret-key: open sesame\n" + baseConfig,
		func(cfg *providerConfig) {
			cfg.auth = aws.Auth{
				AccessKey: "jujuer",
				SecretKey: "open sesame",
			}
		},
		"",
	},
	{
		"authorized-keys: authkeys\n" + baseConfig,
		func(cfg *providerConfig) {
			cfg.authorizedKeys = "authkeys"
		},
		"",
	},
	{
		"access-key: jujuer\n" + baseConfig,
		nil,
		".*environment has access-key but no secret-key",
	},
	{
		"secret-key: badness\n" + baseConfig,
		nil,
		".*environment has secret-key but no access-key",
	},

	// unknown fields are discarded
	{
		"unknown-something: 666\n" + baseConfig,
		func(cfg *providerConfig) {},
		"",
	},
}

func indent(s string, with string) string {
	var r string
	lines := strings.Split(s, "\n")
	for _, l := range lines {
		r += with + l + "\n"
	}
	return r
}

func makeEnv(s string) []byte {
	return []byte("environments:\n  testenv:\n    type: ec2\n" + indent(s, "    "))
}

func (s *configSuite) SetUpTest(c *C) {
	s.home = os.Getenv("HOME")
	s.accessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	s.secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")

	home := c.MkDir()
	sshDir := filepath.Join(home, ".ssh")
	err := os.Mkdir(sshDir, 0777)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(sshDir, "id_rsa.pub"), []byte("sshkey\n"), 0666)
	c.Assert(err, IsNil)

	os.Setenv("HOME", home)
	os.Setenv("AWS_ACCESS_KEY_ID", testAuth.AccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", testAuth.SecretKey)
	Regions["configtest"] = configTestRegion
}

func (s *configSuite) TearDownTest(c *C) {
	os.Setenv("HOME", s.home)
	os.Setenv("AWS_ACCESS_KEY_ID", s.accessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", s.secretKey)
	delete(Regions, "configtest")
}

func (s *configSuite) TestConfig(c *C) {
	for i, t := range configTests {
		c.Logf("test %d (environ %q)", i, t.env)
		t.check(c)
	}
}

func (s *configSuite) TestMissingAuth(c *C) {
	os.Setenv("AWS_ACCESS_KEY_ID", "")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "")
	test := configTests[0]
	test.err = ".*not found in environment"
	test.check(c)
}

func (s *configSuite) TestAuthorizedKeysPath(c *C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "something")
	err := ioutil.WriteFile(path, []byte("another-sshkey\n"), 0666)
	c.Assert(err, IsNil)
	confLine := fmt.Sprintf("authorized-keys-path: %s\n", path)
	test := configTest{
		confLine + baseConfig,
		func(cfg *providerConfig) {
			cfg.authorizedKeys = "another-sshkey\n"
		},
		"",
	}
	test.check(c)
}
