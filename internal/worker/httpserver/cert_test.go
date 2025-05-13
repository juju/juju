// Copyright 2016-2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"crypto/tls"
	"net/http"
	"net/url"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/internal/logger"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/httpserver"
)

type certSuite struct {
	workerFixture
}

var _ = tc.Suite(&certSuite{})

func testSNIGetter(cert *tls.Certificate) httpserver.SNIGetterFunc {
	return func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return cert, nil
	}
}

func (s *certSuite) SetUpTest(c *tc.C) {
	s.workerFixture.SetUpTest(c)
	tlsConfig := httpserver.InternalNewTLSConfig(
		"",
		"https://0.1.2.3/no-autocert-here",
		nil,
		testSNIGetter(coretesting.ServerTLSCert),
		logger.GetLogger("test"),
	)
	// Copy the root CAs across.
	tlsConfig.RootCAs = s.config.TLSConfig.RootCAs
	s.config.TLSConfig = tlsConfig
	s.config.TLSConfig.ServerName = "juju-apiserver"
	_ = s.config.Mux.AddHandler("GET", "/hey", http.HandlerFunc(s.handler))
}

func (s *certSuite) handler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("yay"))
}

func (s *certSuite) TestAutocertFailure(c *tc.C) {
	// We don't have a fake autocert server, but we can at least
	// smoke test that the autocert path is followed when we try
	// to connect to a DNS name - the AutocertURL configured
	// by the testing suite is invalid so it should fail.

	// Dropping the handler returned here disables the challenge
	// listener.
	tlsConfig := httpserver.InternalNewTLSConfig(
		"somewhere.example",
		"https://0.1.2.3/no-autocert-here",
		nil,
		testSNIGetter(coretesting.ServerTLSCert),
		logger.GetLogger("test"),
	)
	s.config.TLSConfig = tlsConfig

	worker, err := httpserver.NewWorker(s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

	parsed, err := url.Parse(worker.URL())
	c.Assert(err, tc.ErrorIsNil)

	entries := gatherLog(func() {
		_, err := tls.Dial("tcp", parsed.Host, &tls.Config{
			ServerName: "somewhere.example",
		})
		expectedErr := `.*x509: certificate is valid for .*, not somewhere.example`
		// We can't get an autocert certificate, so we'll fall back to the local certificate
		// which isn't valid for connecting to somewhere.example.
		c.Assert(err, tc.ErrorMatches, expectedErr)
	})

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	// We will log the failure to get the certificate, thus assuring us that we actually tried.
	c.Assert(entries, mc, []loggo.Entry{{
		Level:   loggo.DEBUG,
		Message: `getting certificate for server name "somewhere.example"`,
	}, {
		Level:   loggo.ERROR,
		Message: `.*getting autocert certificate for "somewhere.example": Get ["]?https://0.1.2.3/no-autocert-here["]?: .*`,
	}})
}

func (s *certSuite) TestAutocertNoAutocertDNSName(c *tc.C) {
	worker, err := httpserver.NewWorker(s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

	parsed, err := url.Parse(worker.URL())
	c.Assert(err, tc.ErrorIsNil)

	entries := gatherLog(func() {
		_, err := tls.Dial("tcp", parsed.Host, &tls.Config{
			ServerName: "somewhere.example",
		})
		expectedErr := `.*x509: certificate is valid for .*, not somewhere.example`
		// We can't get an autocert certificate, so we'll fall back to the local certificate
		// which isn't valid for connecting to somewhere.example.
		c.Assert(err, tc.ErrorMatches, expectedErr)
	})

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	// Check that we never logged a failure to get the certificate.
	c.Assert(entries, tc.Not(mc), []loggo.Entry{{
		Level:   loggo.ERROR,
		Message: `.*cannot get autocert certificate.*`,
	}})
}

func gatherLog(f func()) []loggo.Entry {
	var tw loggo.TestWriter
	err := loggo.RegisterWriter("test", &tw)
	if err != nil {
		panic(err)
	}
	defer func() { _, _ = loggo.RemoveWriter("test") }()
	f()
	return tw.Log()
}
