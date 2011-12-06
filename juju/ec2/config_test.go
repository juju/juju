package ec2

import (
	"launchpad.net/goamz/aws"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
	"os"
	"strings"
)

var testRegion = aws.Region{
	EC2Endpoint: "testregion.nowhere:1234",
}

var defa = aws.Auth{"gopher", "long teeth"}

type configTest struct {
	env    string
	config *providerConfig
	err    string
}

var configTests = []configTest{
	{"", &providerConfig{region: aws.USEast, auth: defa}, ""},
	{"region: eu-west-1\n", &providerConfig{region: aws.EUWest, auth: defa}, ""},
	{"region: unknown\n", nil, ".*invalid region name.*"},
	{"region: test\n", &providerConfig{region: testRegion, auth: defa}, ""},
	{"region: deleted\n", nil, ".*invalid region name.*"},
	{"region: 666\n", nil, ".*expected string, got 666"},
	{"access-key: 666\n", nil, ".*expected string, got 666"},
	{"secret-key: 666\n", nil, ".*expected string, got 666"},
	{"access-key: jujuer\nsecret-key: open sesame\n",
		&providerConfig{
			region: aws.USEast,
			auth: aws.Auth{
				AccessKey: "jujuer",
				SecretKey: "open sesame",
			},
		},
		"",
	},
	{"access-key: jujuer\n", nil, ".*environment has access-key but no secret-key"},
	{"secret-key: badness\n", nil, ".*environment has secret-key but no access-key"},
	// unknown fields are discarded
	{"unknown-something: 666\n", &providerConfig{region: aws.USEast, auth: defa}, ""},
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
	return []byte("environments:\n  test:\n    type: ec2\n" + indent(s, "    "))
}

func (suite) TestConfig(c *C) {
	AddRegion("test", testRegion)
	AddRegion("deleted", aws.Region{S3Endpoint: "shouldneverbeseen.nowhere:1234"})
	RemoveRegion("deleted")

	defer os.Setenv("AWS_ACCESS_KEY_ID", os.Getenv("AWS_ACCESS_KEY_ID"))
	defer os.Setenv("AWS_SECRET_ACCESS_KEY", os.Getenv("AWS_SECRET_ACCESS_KEY"))

	os.Setenv("AWS_ACCESS_KEY_ID", "")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "")

	// first try with no auth environment vars set
	test := configTest{"", &providerConfig{region: aws.USEast, auth: defa}, ".*not found in environment"}
	test.run(c)

	// then set defaults
	os.Setenv("AWS_ACCESS_KEY_ID", defa.AccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", defa.SecretKey)

	for _, t := range configTests {
		t.run(c)
	}
}

func (t configTest) run(c *C) {
	envs, err := juju.ReadEnvironsBytes(makeEnv(t.env))
	if err != nil {
		if t.err != "" {
			c.Check(err, ErrorMatches, t.err, Bug("environ %q", t.env))
		} else {
			c.Check(err, IsNil, Bug("environ %q", t.env))
		}
		return
	}
	e, err := envs.Open("test")
	c.Assert(err, IsNil)
	c.Assert(e, NotNil)
	ec2env, ok := e.(*environ)
	c.Assert(ok, Equals, true, Bug("unexpected type %T, environ %q", ec2env, t.env))
	c.Check(ec2env.config, Equals, t.config, Bug("environ %q", t.env))
}
