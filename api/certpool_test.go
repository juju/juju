// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/testing"
)

type certPoolSuite struct {
	testing.BaseSuite
	logs *certLogs
}

var _ = gc.Suite(&certPoolSuite{})

func (s *certPoolSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.logs = &certLogs{}
	loggo.GetLogger("juju.api").SetLogLevel(loggo.TRACE)
	loggo.RegisterWriter("api-certs", s.logs, loggo.TRACE)
}

func (*certPoolSuite) TestCreateCertPoolNoCert(c *gc.C) {
	pool, err := api.CreateCertPool("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pool.Subjects(), gc.HasLen, 0)
}

func (*certPoolSuite) TestCreateCertPoolTestCert(c *gc.C) {
	pool, err := api.CreateCertPool(testing.CACert)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pool.Subjects(), gc.HasLen, 1)
}

func (s *certPoolSuite) TestCreateCertPoolNoDir(c *gc.C) {
	certDir := filepath.Join(c.MkDir(), "missing")
	s.PatchValue(api.CertDir, certDir)

	pool, err := api.CreateCertPool("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pool.Subjects(), gc.HasLen, 0)

	c.Assert(s.logs.messages, gc.HasLen, 1)
	// The directory not existing is likely to happen a lot, so it is only
	// logged out at trace to help be explicit in the case where detailed
	// debugging is needed.
	c.Assert(s.logs.messages[0], gc.Matches, `TRACE cert dir ".*" does not exist`)
}

func (s *certPoolSuite) TestCreateCertPoolNotADir(c *gc.C) {
	certDir := filepath.Join(c.MkDir(), "missing")
	s.PatchValue(api.CertDir, certDir)
	// Make the certDir a file instead...
	c.Assert(ioutil.WriteFile(certDir, []byte("blah"), 0644), jc.ErrorIsNil)

	pool, err := api.CreateCertPool("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pool.Subjects(), gc.HasLen, 0)

	c.Assert(s.logs.messages, gc.HasLen, 1)
	c.Assert(s.logs.messages[0], gc.Matches, `INFO cert dir ".*" is not a directory`)
}

func (s *certPoolSuite) TestCreateCertPoolEmptyDir(c *gc.C) {
	certDir := c.MkDir()
	s.PatchValue(api.CertDir, certDir)

	pool, err := api.CreateCertPool("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pool.Subjects(), gc.HasLen, 0)
	c.Assert(s.logs.messages, gc.HasLen, 1)
	c.Assert(s.logs.messages[0], gc.Matches, `DEBUG added 0 certs to the pool from .*`)
}

func (s *certPoolSuite) TestCreateCertPoolLoadsPEMFiles(c *gc.C) {
	certDir := c.MkDir()
	s.PatchValue(api.CertDir, certDir)
	s.addCert(c, filepath.Join(certDir, "first.pem"))
	s.addCert(c, filepath.Join(certDir, "second.pem"))
	s.addCert(c, filepath.Join(certDir, "third.pem"))

	pool, err := api.CreateCertPool("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pool.Subjects(), gc.HasLen, 3)
	c.Assert(s.logs.messages, gc.HasLen, 1)
	c.Assert(s.logs.messages[0], gc.Matches, `DEBUG added 3 certs to the pool from .*`)
}

func (s *certPoolSuite) TestCreateCertPoolLoadsOnlyPEMFiles(c *gc.C) {
	certDir := c.MkDir()
	s.PatchValue(api.CertDir, certDir)
	s.addCert(c, filepath.Join(certDir, "first.pem"))
	c.Assert(ioutil.WriteFile(filepath.Join(certDir, "second.cert"), []byte("blah"), 0644), jc.ErrorIsNil)

	pool, err := api.CreateCertPool("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pool.Subjects(), gc.HasLen, 1)
	c.Assert(s.logs.messages, gc.HasLen, 1)
	c.Assert(s.logs.messages[0], gc.Matches, `DEBUG added 1 certs to the pool from .*`)
}

func (s *certPoolSuite) TestCreateCertPoolLogsBadCerts(c *gc.C) {
	certDir := c.MkDir()
	s.PatchValue(api.CertDir, certDir)
	c.Assert(ioutil.WriteFile(filepath.Join(certDir, "broken.pem"), []byte("blah"), 0644), jc.ErrorIsNil)

	pool, err := api.CreateCertPool("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pool.Subjects(), gc.HasLen, 0)
	c.Assert(s.logs.messages, gc.HasLen, 2)
	c.Assert(s.logs.messages[0], gc.Matches, `INFO error parsing cert ".*broken.pem": .*`)
	c.Assert(s.logs.messages[1], gc.Matches, `DEBUG added 0 certs to the pool from .*`)
}

func (s *certPoolSuite) addCert(c *gc.C, filename string) {
	expiry := time.Now().UTC().AddDate(10, 0, 0)
	pem, _, err := cert.NewCA("random env name", expiry)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filename, []byte(pem), 0644)
	c.Assert(err, jc.ErrorIsNil)
}

type certLogs struct {
	messages []string
}

func (c *certLogs) Write(level loggo.Level, name, filename string, line int, timestamp time.Time, message string) {
	if strings.HasSuffix(filename, "certpool.go") {
		c.messages = append(c.messages, fmt.Sprintf("%s %s", level, message))
	}
}
