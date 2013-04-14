package environs_test

import (
	. "launchpad.net/gocheck"
)

type CloudInitSuite struct{}

var _ = Suite(&CloudInitSuite{})

func (s *CloudInitSuite) TestFatal(c *C) {
	c.Fatalf("lots to write: keygen etc")
	/*
	-	caCertPEM, err := ioutil.ReadFile(config.JujuHomePath("foo-cert.pem"))
	-	c.Assert(err, IsNil)
	-
	-	err = cert.Verify(env.certPEM, caCertPEM, time.Now())
	-	c.Assert(err, IsNil)
	-	err = cert.Verify(env.certPEM, caCertPEM, time.Now().AddDate(9, 0, 0))
	-	c.Assert(err, IsNil)
	*/
}
