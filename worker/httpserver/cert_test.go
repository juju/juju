// Copyright 2016-2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"net/url"
	"runtime"
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/cert"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/httpserver"
)

type certSuite struct {
	workerFixture

	cert *tls.Certificate
}

var _ = gc.Suite(&certSuite{})

func (s *certSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)
	tlsConfig := httpserver.InternalNewTLSConfig(
		"",
		"https://0.1.2.3/no-autocert-here",
		nil,
		func() *tls.Certificate { return s.cert },
	)
	// Copy the root CAs across.
	tlsConfig.RootCAs = s.config.TLSConfig.RootCAs
	s.config.TLSConfig = tlsConfig
	s.config.TLSConfig.ServerName = "juju-apiserver"
	s.config.Mux.AddHandler("GET", "/hey", http.HandlerFunc(s.handler))
	s.cert = coretesting.ServerTLSCert
}

func (s *certSuite) handler(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("yay"))
}

func (s *certSuite) request(url string) (*http.Response, error) {
	// Create the client each time to ensure that we get the
	// certificate again.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: s.config.TLSConfig,
		},
	}
	return client.Get(url)
}

func (s *certSuite) TestUpdateCert(c *gc.C) {
	worker, err := httpserver.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

	url := worker.URL() + "/hey"
	// Sanity check that the server works initially.
	resp, err := s.request(url)
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	content, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, "yay")

	// Create a new certificate that's a year out of date, so we can
	// tell that the server is using it because the connection will fail.
	srvCert, srvKey, err := cert.NewServer(coretesting.CACert, coretesting.CAKey, time.Now().AddDate(-1, 0, 0), nil)
	c.Assert(err, jc.ErrorIsNil)
	badTLSCert, err := tls.X509KeyPair([]byte(srvCert), []byte(srvKey))
	if err != nil {
		panic(err)
	}
	x509Cert, err := x509.ParseCertificate(badTLSCert.Certificate[0])
	if err != nil {
		panic(err)
	}
	badTLSCert.Leaf = x509Cert

	// Check that we can't connect to the server because of the bad certificate.
	s.cert = &badTLSCert
	_, err = s.request(url)
	c.Assert(err, gc.ErrorMatches, `.*: certificate has expired or is not yet valid.*`)

	// Replace the working certificate and check that we can connect again.
	s.cert = coretesting.ServerTLSCert
	resp, err = s.request(url)
	c.Assert(err, jc.ErrorIsNil)
	resp.Body.Close()
}

func (s *certSuite) TestAutocertFailure(c *gc.C) {
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
		func() *tls.Certificate { return s.cert },
	)
	s.config.TLSConfig = tlsConfig

	worker, err := httpserver.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

	parsed, err := url.Parse(worker.URL())
	c.Assert(err, jc.ErrorIsNil)

	entries := gatherLog(func() {
		_, err := tls.Dial("tcp", parsed.Host, &tls.Config{
			ServerName: "somewhere.example",
		})
		expectedErr := `x509: certificate is valid for \*, not somewhere.example`
		if runtime.GOOS == "windows" {
			// For some reason, windows doesn't think that the certificate is signed
			// by a valid authority. This could be problematic.
			expectedErr = "x509: certificate signed by unknown authority"
		}
		// We can't get an autocert certificate, so we'll fall back to the local certificate
		// which isn't valid for connecting to somewhere.example.
		c.Assert(err, gc.ErrorMatches, expectedErr)
	})
	// We will log the failure to get the certificate, thus assuring us that we actually tried.
	c.Assert(entries, jc.LogMatches, jc.SimpleMessages{{
		loggo.ERROR,
		`.*cannot get autocert certificate for "somewhere.example".*`,
	}})
}

func (s *certSuite) TestAutocertNameMismatch(c *gc.C) {
	tlsConfig := httpserver.InternalNewTLSConfig(
		"somewhere.example",
		"https://0.1.2.3/no-autocert-here",
		nil,
		func() *tls.Certificate { return s.cert },
	)
	s.config.TLSConfig = tlsConfig

	worker, err := httpserver.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

	parsed, err := url.Parse(worker.URL())
	c.Assert(err, jc.ErrorIsNil)

	entries := gatherLog(func() {
		_, err := tls.Dial("tcp", parsed.Host, &tls.Config{
			ServerName: "somewhere.else",
		})
		expectedErr := `x509: certificate is valid for \*, not somewhere.else`
		if runtime.GOOS == "windows" {
			// For some reason, windows doesn't think that the certificate is signed
			// by a valid authority. This could be problematic.
			expectedErr = "x509: certificate signed by unknown authority"
		}
		// We can't get an autocert certificate, so we'll fall back to the local certificate
		// which isn't valid for connecting to somewhere.example.
		c.Assert(err, gc.ErrorMatches, expectedErr)
	})
	// Check that we logged the mismatch.
	c.Assert(entries, jc.LogMatches, jc.SimpleMessages{{
		loggo.ERROR,
		`.*cannot get autocert certificate for "somewhere.else": acme/autocert: host "somewhere.else" not configured in HostWhitelist`,
	}})
}

func (s *certSuite) TestAutocertNoAutocertDNSName(c *gc.C) {
	worker, err := httpserver.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

	parsed, err := url.Parse(worker.URL())
	c.Assert(err, jc.ErrorIsNil)

	entries := gatherLog(func() {
		_, err := tls.Dial("tcp", parsed.Host, &tls.Config{
			ServerName: "somewhere.example",
		})
		expectedErr := `x509: certificate is valid for \*, not somewhere.example`
		if runtime.GOOS == "windows" {
			// For some reason, windows doesn't think that the certificate is signed
			// by a valid authority. This could be problematic.
			expectedErr = "x509: certificate signed by unknown authority"
		}
		// We can't get an autocert certificate, so we'll fall back to the local certificate
		// which isn't valid for connecting to somewhere.example.
		c.Assert(err, gc.ErrorMatches, expectedErr)
	})
	// Check that we never logged a failure to get the certificate.
	c.Assert(entries, gc.Not(jc.LogMatches), jc.SimpleMessages{{
		loggo.ERROR,
		`.*cannot get autocert certificate.*`,
	}})
}

func gatherLog(f func()) []loggo.Entry {
	var tw loggo.TestWriter
	err := loggo.RegisterWriter("test", &tw)
	if err != nil {
		panic(err)
	}
	defer loggo.RemoveWriter("test")
	f()
	return tw.Log()
}
