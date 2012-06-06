package ec2_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/environs/ec2"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type authSuite struct{}

func blankLine(k string) bool {
	// TODO treat "\r" as blank?
	return len(k) == 0 || k[0] == '#'
}

func parseKeys(keys string) []string {
	ks := strings.Split(keys, "\n")
	i := 0
	for _, k := range ks {
		if blankLine(k) {
			continue
		}
		ks[i] = k
		i++
	}
	return ks[0:i]
}

func (authSuite) TestAuthorizedKeys(c *C) {
	d := c.MkDir()
	defer os.Setenv("HOME", os.Getenv("HOME"))
	os.Setenv("HOME", d)

	d = filepath.Join(d, ".ssh")

	err := os.Mkdir(d, 0777)
	c.Assert(err, IsNil)

	keys, err := ec2.AuthorizedKeys("")
	c.Assert(err, ErrorMatches, "no keys found")

	err = ioutil.WriteFile(filepath.Join(d, "id_dsa.pub"), []byte("dsa"), 0666)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(d, "id_rsa.pub"), []byte("rsa\n"), 0666)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(d, "identity.pub"), []byte("identity"), 0666)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(d, "authorized_keys"), []byte("auth0\n# first\nauth1\n\n"), 0666)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(d, "authorized_keys2"), []byte("auth2\n"), 0666)
	c.Assert(err, IsNil)

	keys, err = ec2.AuthorizedKeys("")
	c.Assert(err, IsNil)

	ks := strings.Split(keys, "\n")
	sort.Strings(ks)
	c.Check(ks, DeepEquals, []string{
		"",
		"",
		"# first",
		"auth0",
		"auth1",
		"dsa",
		"identity",
		"rsa",
	})

	// explicit path relative to home/.ssh
	keys, err = ec2.AuthorizedKeys("authorized_keys2")
	c.Check(err, IsNil)
	c.Check(keys, Equals, "auth2\n")

	// explicit path relative to home
	keys, err = ec2.AuthorizedKeys(filepath.Join("~", ".ssh", "authorized_keys2"))
	c.Check(err, IsNil)
	c.Check(keys, Equals, "auth2\n")

	// explicit absolute path
	keys, err = ec2.AuthorizedKeys(filepath.Join(d, "authorized_keys2"))
	c.Check(err, IsNil)
	c.Check(keys, Equals, "auth2\n")
}
