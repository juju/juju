package ec2

import (
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
	savedHome, savedAccessKey, savedSecretKey string
}

var _ = Suite(configSuite{})

var configTestRegion = aws.Region{
	Name:        "configtest",
	EC2Endpoint: "testregion.nowhere:1234",
}

var testAuth = aws.Auth{"gopher", "long teeth"}

// the mandatory fields in config.
var baseConfig = "control-bucket: x\n"

// configTest specifies a config parsing test, checking that env when
// parsed as the ec2 section of a config file matches baseConfigResult
// when mutated by the mutate function, or that the parse matches the
// given error.
type configTest struct {
	yaml      string
	region    string
	bucket    string
	pbucket   string
	accessKey string
	secretKey string
	err       string
}

func (t configTest) check(c *C) {
	envs, err := environs.ReadEnvironsBytes(makeEnv(t.yaml))
	if t.err != "" {
		c.Check(err, ErrorMatches, t.err)
		return
	}
	c.Check(err, IsNil)

	e, err := envs.Open("testenv")
	c.Assert(err, IsNil)
	c.Assert(e, NotNil)
	c.Assert(e, FitsTypeOf, (*environ)(nil))

	config := e.(*environ).config()
	c.Assert(config.Name(), Equals, "testenv")
	c.Assert(config.bucket, Equals, "x")
	if t.region != "" {
		c.Assert(config.region, Equals, t.region)
	}
	if t.pbucket != "" {
		c.Assert(config.publicBucket, Equals, t.pbucket)
	}
	if t.accessKey != "" {
		auth := aws.Auth{t.accessKey, t.secretKey}
		c.Assert(config.auth, Equals, auth)
	} else {
		c.Assert(config.auth, DeepEquals, testAuth)
	}
}

var configTests = []configTest{
	{
		yaml: baseConfig,
	},
	{
		yaml:   "region: eu-west-1\n" + baseConfig,
		region: "eu-west-1",
	},
	{
		yaml: "region: unknown\n" + baseConfig,
		err:  ".*invalid region name.*",
	},
	{
		yaml:   "region: configtest\n" + baseConfig,
		region: "configtest",
	},
	{
		yaml: "region: 666\n" + baseConfig,
		err:  ".*expected string, got 666",
	},
	{
		yaml: "access-key: 666\n" + baseConfig,
		err:  ".*expected string, got 666",
	},
	{
		yaml: "secret-key: 666\n" + baseConfig,
		err:  ".*expected string, got 666",
	},
	{
		yaml: "control-bucket: 666\n",
		err:  ".*expected string, got 666",
	},
	{
		yaml: "public-bucket: 666\n" + baseConfig,
		err:  ".*expected string, got 666",
	},
	{
		yaml:    "public-bucket: foo\n" + baseConfig,
		pbucket: "foo",
	},
	{
		yaml:      "access-key: jujuer\nsecret-key: open sesame\n" + baseConfig,
		accessKey: "jujuer",
		secretKey: "open sesame",
	},
	{
		yaml: "access-key: jujuer\n" + baseConfig,
		err:  ".*environment has access-key but no secret-key",
	},
	{
		yaml: "secret-key: badness\n" + baseConfig,
		err:  ".*environment has secret-key but no access-key",
	},

	// unknown fields are discarded
	{
		yaml: "unknown-something: 666\n" + baseConfig,
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
	s.savedHome = os.Getenv("HOME")
	s.savedAccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	s.savedSecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")

	home := c.MkDir()
	sshDir := filepath.Join(home, ".ssh")
	err := os.Mkdir(sshDir, 0777)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(sshDir, "id_rsa.pub"), []byte("sshkey\n"), 0666)
	c.Assert(err, IsNil)

	os.Setenv("HOME", home)
	os.Setenv("AWS_ACCESS_KEY_ID", testAuth.AccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", testAuth.SecretKey)
	aws.Regions["configtest"] = configTestRegion
}

func (s *configSuite) TearDownTest(c *C) {
	os.Setenv("HOME", s.savedHome)
	os.Setenv("AWS_ACCESS_KEY_ID", s.savedAccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", s.savedSecretKey)
	delete(aws.Regions, "configtest")
}

func (s *configSuite) TestConfig(c *C) {
	for i, t := range configTests {
		c.Logf("test %d", i)
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
