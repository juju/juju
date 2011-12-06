package ec2

import (
	"launchpad.net/goamz/aws"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
	"strings"
)

var testRegion = aws.Region{
	EC2Endpoint: "testregion.nowhere:1234",
}

var configTests = []struct {
	env    string
	config *providerConfig
	err    string
}{
	{"type: ec2\n", &providerConfig{region: aws.USEast}, ""},
	{"type: ec2\nregion: eu-west-1\n", &providerConfig{region: aws.EUWest}, ""},
	{"type: ec2\nregion: unknown\n", nil, ".*invalid region name.*"},
	{"type: ec2\nregion: test\n", &providerConfig{region: testRegion}, ""},
	{"type: ec2\nregion: 666\n", nil, ".*expected string, got 666"},
}

func indent(s string, with string) string {
	var r string
	lines := strings.Split(s, "\n")
	for _, l := range lines {
		r += with + l + "\n"
	}
	return r
}

func (suite) TestRegion(c *C) {
	Regions["test"] = testRegion
	for _, t := range configTests {
		env := "environments:\n  test:\n" + indent(t.env, "    ")
		envs, err := juju.ReadEnvironsBytes([]byte(env))
		if err != nil {
			if t.err != "" {
				c.Check(err, ErrorMatches, t.err, Bug("environ %q", t.env))
			} else {
				c.Check(err, IsNil, Bug("environ %q", t.env))
			}
			continue
		}
		e, err := envs.Open("test")
		c.Assert(err, IsNil)
		c.Assert(e, NotNil)
		ec2env, ok := e.(*environ)
		c.Assert(ok, Equals, true, Bug("unexpected type %T, environ %q", ec2env, t.env))
		c.Check(ec2env.config, Equals, t.config)
	}
}
