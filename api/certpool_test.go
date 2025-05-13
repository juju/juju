// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/testing"
)

type certPoolSuite struct {
	testing.BaseSuite
	logs *certLogs
}

var _ = tc.Suite(&certPoolSuite{})

func (s *certPoolSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.logs = &certLogs{}
	oldLevel := loggo.GetLogger("juju.api").LogLevel()
	loggo.GetLogger("juju.api").SetLogLevel(loggo.TRACE)
	loggo.RegisterWriter("api-certs", s.logs)
	c.Cleanup(func() {
		loggo.GetLogger("juju.api").SetLogLevel(oldLevel)
		loggo.RemoveWriter("api-certs")
	})
}

func (*certPoolSuite) TestCreateCertPoolNoCert(c *tc.C) {
	pool, err := api.CreateCertPool("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pool.Subjects(), tc.HasLen, 0)
}

func (*certPoolSuite) TestCreateCertPoolTestCert(c *tc.C) {
	pool, err := api.CreateCertPool(testing.CACert)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pool.Subjects(), tc.HasLen, 1)
}

func (s *certPoolSuite) TestCreateCertPoolNoDir(c *tc.C) {
	certDir := filepath.Join(c.MkDir(), "missing")
	s.PatchValue(api.CertDir, certDir)

	pool, err := api.CreateCertPool("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pool.Subjects(), tc.HasLen, 0)

	c.Assert(s.logs.messages, tc.HasLen, 1)
	// The directory not existing is likely to happen a lot, so it is only
	// logged out at trace to help be explicit in the case where detailed
	// debugging is needed.
	c.Assert(s.logs.messages[0], tc.Matches, `TRACE cert dir ".*" does not exist`)
}

func (s *certPoolSuite) TestCreateCertPoolNotADir(c *tc.C) {
	certDir := filepath.Join(c.MkDir(), "missing")
	s.PatchValue(api.CertDir, certDir)
	// Make the certDir a file instead...
	c.Assert(os.WriteFile(certDir, []byte("blah"), 0644), tc.ErrorIsNil)

	pool, err := api.CreateCertPool("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pool.Subjects(), tc.HasLen, 0)

	c.Assert(s.logs.messages, tc.HasLen, 1)
	c.Assert(s.logs.messages[0], tc.Matches, `INFO cert dir ".*" is not a directory`)
}

func (s *certPoolSuite) TestCreateCertPoolEmptyDir(c *tc.C) {
	certDir := c.MkDir()
	s.PatchValue(api.CertDir, certDir)

	pool, err := api.CreateCertPool("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pool.Subjects(), tc.HasLen, 0)
	c.Assert(s.logs.messages, tc.HasLen, 1)
	c.Assert(s.logs.messages[0], tc.Matches, `DEBUG added 0 certs to the pool from .*`)
}

func (s *certPoolSuite) TestCreateCertPoolLoadsPEMFiles(c *tc.C) {
	certDir := c.MkDir()
	s.PatchValue(api.CertDir, certDir)
	s.addCert(c, filepath.Join(certDir, "first.pem"))
	s.addCert(c, filepath.Join(certDir, "second.pem"))
	s.addCert(c, filepath.Join(certDir, "third.pem"))

	pool, err := api.CreateCertPool("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pool.Subjects(), tc.HasLen, 3)
	c.Assert(s.logs.messages, tc.HasLen, 1)
	c.Assert(s.logs.messages[0], tc.Matches, `DEBUG added 3 certs to the pool from .*`)
}

func (s *certPoolSuite) TestCreateCertPoolLoadsOnlyPEMFiles(c *tc.C) {
	certDir := c.MkDir()
	s.PatchValue(api.CertDir, certDir)
	s.addCert(c, filepath.Join(certDir, "first.pem"))
	c.Assert(os.WriteFile(filepath.Join(certDir, "second.cert"), []byte("blah"), 0644), tc.ErrorIsNil)

	pool, err := api.CreateCertPool("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pool.Subjects(), tc.HasLen, 1)
	c.Assert(s.logs.messages, tc.HasLen, 1)
	c.Assert(s.logs.messages[0], tc.Matches, `DEBUG added 1 certs to the pool from .*`)
}

func (s *certPoolSuite) TestCreateCertPoolLogsBadCerts(c *tc.C) {
	certDir := c.MkDir()
	s.PatchValue(api.CertDir, certDir)
	c.Assert(os.WriteFile(filepath.Join(certDir, "broken.pem"), []byte("blah"), 0644), tc.ErrorIsNil)

	pool, err := api.CreateCertPool("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pool.Subjects(), tc.HasLen, 0)
	c.Assert(s.logs.messages, tc.HasLen, 2)
	c.Assert(s.logs.messages[0], tc.Matches, `INFO error parsing cert ".*broken.pem": .*`)
	c.Assert(s.logs.messages[1], tc.Matches, `DEBUG added 0 certs to the pool from .*`)
}

func (s *certPoolSuite) addCert(c *tc.C, filename string) {
	signer, err := pki.DefaultKeyProfile()
	c.Assert(err, tc.ErrorIsNil)

	caCert, err := pki.NewCA("random model name", signer)
	c.Assert(err, tc.ErrorIsNil)

	caCertPem, err := pki.CertificateToPemString(pki.DefaultPemHeaders, caCert)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filename, []byte(caCertPem), 0644)
	c.Assert(err, tc.ErrorIsNil)
}

type certLogs struct {
	messages []string
}

func (c *certLogs) Write(entry loggo.Entry) {
	if strings.HasSuffix(entry.Filename, "certpool.go") {
		c.messages = append(c.messages, fmt.Sprintf("%s %s", entry.Level, entry.Message))
	}
}
