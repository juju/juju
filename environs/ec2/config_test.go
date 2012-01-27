package ec2

import (
	"launchpad.net/goamz/aws"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	"os"
	"strings"
)

// Use local suite since this file lives in the ec2 package
// for testing internals.
type configSuite struct{}

var _ = Suite(configSuite{})

var configTestRegion = aws.Region{
	EC2Endpoint: "testregion.nowhere:1234",
}

var testAuth = aws.Auth{"gopher", "long teeth"}

type configTest struct {
	env    string
	config *providerConfig
	err    string
}

var configTests = []configTest{
	{
		"control-bucket: x\n",
		&providerConfig{region: "us-east-1", auth: testAuth, bucket: "x"},
		"",
	},
	{
		"region: eu-west-1\ncontrol-bucket: x\n",
		&providerConfig{region: "eu-west-1", auth: testAuth, bucket: "x"},
		"",
	},
	{
		"region: unknown\ncontrol-bucket: x\n",
		nil,
		".*invalid region name.*",
	},
	{
		"region: configtest\ncontrol-bucket: x\n",
		&providerConfig{region: "configtest", auth: testAuth, bucket: "x"},
		"",
	},
	{
		"region: 666\ncontrol-bucket: x\n",
		nil,
		".*expected string, got 666",
	},
	{
		"access-key: 666\ncontrol-bucket: x\n",
		nil,
		".*expected string, got 666",
	},
	{
		"secret-key: 666\ncontrol-bucket: x\n",
		nil,
		".*expected string, got 666",
	},
	{
		"control-bucket: 666\n",
		nil,
		".*expected string, got 666",
	},
	{
		"access-key: jujuer\nsecret-key: open sesame\ncontrol-bucket: x\n",
		&providerConfig{
			region: "us-east-1",
			auth: aws.Auth{
				AccessKey: "jujuer",
				SecretKey: "open sesame",
			},
			bucket: "x",
		},
		"",
	},
	{
		"access-key: jujuer\ncontrol-bucket: x\n",
		nil,
		".*environment has access-key but no secret-key",
	},
	{
		"secret-key: badness\ncontrol-bucket: x\n",
		nil,
		".*environment has secret-key but no access-key",
	},
	// unknown fields are discarded
	{
		"unknown-something: 666\ncontrol-bucket: x",
		&providerConfig{region: "us-east-1", auth: testAuth, bucket: "x"},
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

func (configSuite) TestConfig(c *C) {
	Regions["configtest"] = configTestRegion
	defer delete(Regions, "configtest")

	defer os.Setenv("AWS_ACCESS_KEY_ID", os.Getenv("AWS_ACCESS_KEY_ID"))
	defer os.Setenv("AWS_SECRET_ACCESS_KEY", os.Getenv("AWS_SECRET_ACCESS_KEY"))

	os.Setenv("AWS_ACCESS_KEY_ID", "")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "")

	// first try with no auth environment vars set
	test := configTests[0]
	test.err = ".*not found in environment"
	test.run(c)

	// then set testAuthults
	os.Setenv("AWS_ACCESS_KEY_ID", testAuth.AccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", testAuth.SecretKey)

	for _, t := range configTests {
		t.run(c)
	}
}

func (t configTest) run(c *C) {
	envs, err := environs.ReadEnvironsBytes(makeEnv(t.env))
	if err != nil {
		if t.err != "" {
			c.Check(err, ErrorMatches, t.err, Bug("environ %q", t.env))
		} else {
			c.Check(err, IsNil, Bug("environ %q", t.env))
		}
		return
	}
	e, err := envs.Open("testenv")
	c.Assert(err, IsNil)
	c.Assert(e, NotNil)
	c.Assert(e, FitsTypeOf, (*environ)(nil), Bug("environ %q", t.env))
	c.Check(e.(*environ).config, Equals, t.config, Bug("environ %q", t.env))
}
