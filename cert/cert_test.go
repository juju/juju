// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cert_test

import (
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cert"
)

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

type certSuite struct{}

var _ = gc.Suite(certSuite{})

func (certSuite) TestParseCertificate(c *gc.C) {
	xcert, err := cert.ParseCert(caCertPEM)
	c.Assert(err, gc.IsNil)
	c.Assert(xcert.Subject.CommonName, gc.Equals, "juju testing")

	xcert, err = cert.ParseCert(caKeyPEM)
	c.Check(xcert, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "no certificates found")

	xcert, err = cert.ParseCert([]byte("hello"))
	c.Check(xcert, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "no certificates found")
}

func (certSuite) TestParseCertAndKey(c *gc.C) {
	xcert, key, err := cert.ParseCertAndKey(caCertPEM, caKeyPEM)
	c.Assert(err, gc.IsNil)
	c.Assert(xcert.Subject.CommonName, gc.Equals, "juju testing")
	c.Assert(key, gc.NotNil)

	c.Assert(xcert.PublicKey.(*rsa.PublicKey), gc.DeepEquals, &key.PublicKey)
}

func (certSuite) TestNewCA(c *gc.C) {
	expiry := roundTime(time.Now().AddDate(0, 0, 1))
	caCertPEM, caKeyPEM, err := cert.NewCA("foo", expiry)
	c.Assert(err, gc.IsNil)

	caCert, caKey, err := cert.ParseCertAndKey(caCertPEM, caKeyPEM)
	c.Assert(err, gc.IsNil)

	c.Assert(caKey, gc.FitsTypeOf, (*rsa.PrivateKey)(nil))
	c.Assert(caCert.Subject.CommonName, gc.Equals, `juju-generated CA for environment "foo"`)
	c.Assert(caCert.NotAfter.Equal(expiry), gc.Equals, true)
	c.Assert(caCert.BasicConstraintsValid, gc.Equals, true)
	c.Assert(caCert.IsCA, gc.Equals, true)
	//c.Assert(caCert.MaxPathLen, Equals, 0)	TODO it ends up as -1 - check that this is ok.
}

func (certSuite) TestNewServer(c *gc.C) {
	expiry := roundTime(time.Now().AddDate(1, 0, 0))
	caCertPEM, caKeyPEM, err := cert.NewCA("foo", expiry)
	c.Assert(err, gc.IsNil)

	caCert, _, err := cert.ParseCertAndKey(caCertPEM, caKeyPEM)
	c.Assert(err, gc.IsNil)

	var noHostnames []string
	srvCertPEM, srvKeyPEM, err := cert.NewServer(caCertPEM, caKeyPEM, expiry, noHostnames)
	c.Assert(err, gc.IsNil)

	srvCert, srvKey, err := cert.ParseCertAndKey(srvCertPEM, srvKeyPEM)
	c.Assert(err, gc.IsNil)
	c.Assert(srvCert.Subject.CommonName, gc.Equals, "*")
	c.Assert(srvCert.NotAfter.Equal(expiry), gc.Equals, true)
	c.Assert(srvCert.BasicConstraintsValid, gc.Equals, false)
	c.Assert(srvCert.IsCA, gc.Equals, false)
	c.Assert(srvCert.ExtKeyUsage, gc.DeepEquals, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})

	checkTLSConnection(c, caCert, srvCert, srvKey)
}

func (certSuite) TestNewServerHostnames(c *gc.C) {
	type test struct {
		hostnames           []string
		expectedDNSNames    []string
		expectedIPAddresses []net.IP
	}
	tests := []test{{
		[]string{},
		nil,
		nil,
	}, {
		[]string{"example.com"},
		[]string{"example.com"},
		nil,
	}, {
		[]string{"example.com", "127.0.0.1"},
		[]string{"example.com"},
		[]net.IP{net.IPv4(127, 0, 0, 1).To4()},
	}, {
		[]string{"::1"},
		nil,
		[]net.IP{net.IPv6loopback},
	}}
	for i, t := range tests {
		c.Logf("test %d: %v", i, t.hostnames)
		expiry := roundTime(time.Now().AddDate(1, 0, 0))
		srvCertPEM, srvKeyPEM, err := cert.NewServer(caCertPEM, caKeyPEM, expiry, t.hostnames)
		c.Assert(err, gc.IsNil)
		srvCert, _, err := cert.ParseCertAndKey(srvCertPEM, srvKeyPEM)
		c.Assert(err, gc.IsNil)
		c.Assert(srvCert.DNSNames, gc.DeepEquals, t.expectedDNSNames)
		c.Assert(srvCert.IPAddresses, gc.DeepEquals, t.expectedIPAddresses)
	}
}

func (certSuite) TestWithNonUTCExpiry(c *gc.C) {
	expiry, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", "2012-11-28 15:53:57 +0100 CET")
	c.Assert(err, gc.IsNil)
	certPEM, keyPEM, err := cert.NewCA("foo", expiry)
	xcert, err := cert.ParseCert(certPEM)
	c.Assert(err, gc.IsNil)
	c.Assert(xcert.NotAfter.Equal(expiry), gc.Equals, true)

	var noHostnames []string
	certPEM, _, err = cert.NewServer(certPEM, keyPEM, expiry, noHostnames)
	xcert, err = cert.ParseCert(certPEM)
	c.Assert(err, gc.IsNil)
	c.Assert(xcert.NotAfter.Equal(expiry), gc.Equals, true)
}

func (certSuite) TestNewServerWithInvalidCert(c *gc.C) {
	var noHostnames []string
	srvCert, srvKey, err := cert.NewServer(nonCACert, nonCAKey, time.Now(), noHostnames)
	c.Check(srvCert, gc.IsNil)
	c.Check(srvKey, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "CA certificate is not a valid CA")
}

func (certSuite) TestVerify(c *gc.C) {
	now := time.Now()
	caCert, caKey, err := cert.NewCA("foo", now.Add(1*time.Minute))
	c.Assert(err, gc.IsNil)

	var noHostnames []string
	srvCert, _, err := cert.NewServer(caCert, caKey, now.Add(3*time.Minute), noHostnames)
	c.Assert(err, gc.IsNil)

	err = cert.Verify(srvCert, caCert, now)
	c.Assert(err, gc.IsNil)

	err = cert.Verify(srvCert, caCert, now.Add(55*time.Second))
	c.Assert(err, gc.IsNil)

	// TODO(rog) why does this succeed?
	// err = cert.Verify(srvCert, caCert, now.Add(-1 * time.Minute))
	//c.Check(err, gc.ErrorMatches, "x509: certificate has expired or is not yet valid")

	err = cert.Verify(srvCert, caCert, now.Add(2*time.Minute))
	c.Check(err, gc.ErrorMatches, "x509: certificate has expired or is not yet valid")

	caCert2, caKey2, err := cert.NewCA("bar", now.Add(1*time.Minute))
	c.Assert(err, gc.IsNil)

	// Check original server certificate against wrong CA.
	err = cert.Verify(srvCert, caCert2, now)
	c.Check(err, gc.ErrorMatches, "x509: certificate signed by unknown authority")

	srvCert2, _, err := cert.NewServer(caCert2, caKey2, now.Add(1*time.Minute), noHostnames)
	c.Assert(err, gc.IsNil)

	// Check new server certificate against original CA.
	err = cert.Verify(srvCert2, caCert, now)
	c.Check(err, gc.ErrorMatches, "x509: certificate signed by unknown authority")
}

// checkTLSConnection checks that we can correctly perform a TLS
// handshake using the given credentials.
func checkTLSConnection(c *gc.C, caCert, srvCert *x509.Certificate, srvKey *rsa.PrivateKey) (caName string) {
	clientCertPool := x509.NewCertPool()
	clientCertPool.AddCert(caCert)

	var inBytes, outBytes bytes.Buffer

	const msg = "hello to the server"
	p0, p1 := net.Pipe()
	p0 = bufferedConn(p0, 3)
	p0 = recordingConn(p0, &inBytes, &outBytes)

	var clientState tls.ConnectionState
	done := make(chan error)
	go func() {
		clientConn := tls.Client(p0, &tls.Config{
			ServerName: "anyServer",
			RootCAs:    clientCertPool,
		})
		defer clientConn.Close()

		_, err := clientConn.Write([]byte(msg))
		if err != nil {
			done <- fmt.Errorf("client: %v", err)
		}
		clientState = clientConn.ConnectionState()
		done <- nil
	}()
	go func() {
		serverConn := tls.Server(p1, &tls.Config{
			Certificates: []tls.Certificate{
				newTLSCert(c, srvCert, srvKey),
			},
		})
		defer serverConn.Close()
		data, err := ioutil.ReadAll(serverConn)
		if err != nil {
			done <- fmt.Errorf("server: %v", err)
			return
		}
		if string(data) != msg {
			done <- fmt.Errorf("server: got %q; expected %q", data, msg)
			return
		}

		done <- nil
	}()

	for i := 0; i < 2; i++ {
		err := <-done
		c.Check(err, gc.IsNil)
	}

	outData := string(outBytes.Bytes())
	c.Assert(outData, gc.Not(gc.HasLen), 0)
	if strings.Index(outData, msg) != -1 {
		c.Fatalf("TLS connection not encrypted")
	}
	c.Assert(clientState.VerifiedChains, gc.HasLen, 1)
	c.Assert(clientState.VerifiedChains[0], gc.HasLen, 2)
	return clientState.VerifiedChains[0][1].Subject.CommonName
}

func newTLSCert(c *gc.C, cert *x509.Certificate, key *rsa.PrivateKey) tls.Certificate {
	return tls.Certificate{
		Certificate: [][]byte{cert.Raw},
		PrivateKey:  key,
	}
}

// bufferedConn adds buffering for at least
// n writes to the given connection.
func bufferedConn(c net.Conn, n int) net.Conn {
	for i := 0; i < n; i++ {
		p0, p1 := net.Pipe()
		go copyClose(p1, c)
		go copyClose(c, p1)
		c = p0
	}
	return c
}

// recordingConn returns a connection which
// records traffic in or out of the given connection.
func recordingConn(c net.Conn, in, out io.Writer) net.Conn {
	p0, p1 := net.Pipe()
	go func() {
		io.Copy(io.MultiWriter(c, out), p1)
		c.Close()
	}()
	go func() {
		io.Copy(io.MultiWriter(p1, in), c)
		p1.Close()
	}()
	return p0
}

func copyClose(w io.WriteCloser, r io.Reader) {
	io.Copy(w, r)
	w.Close()
}

// roundTime returns t rounded to the previous whole second.
func roundTime(t time.Time) time.Time {
	return t.Add(time.Duration(-t.Nanosecond()))
}

var (
	caCertPEM = []byte(`
-----BEGIN CERTIFICATE-----
MIIBnTCCAUmgAwIBAgIBADALBgkqhkiG9w0BAQUwJjENMAsGA1UEChMEanVqdTEV
MBMGA1UEAxMManVqdSB0ZXN0aW5nMB4XDTEyMTExNDE0Mzg1NFoXDTIyMTExNDE0
NDM1NFowJjENMAsGA1UEChMEanVqdTEVMBMGA1UEAxMManVqdSB0ZXN0aW5nMFow
CwYJKoZIhvcNAQEBA0sAMEgCQQCCOOpn9aWKcKr2GQGtygwD7PdfNe1I9BYiPAqa
2I33F5+6PqFdfujUKvoyTJI6XG4Qo/CECaaN9smhyq9DxzMhAgMBAAGjZjBkMA4G
A1UdDwEB/wQEAwIABDASBgNVHRMBAf8ECDAGAQH/AgEBMB0GA1UdDgQWBBQQDswP
FQGeGMeTzPbHW62EZbbTJzAfBgNVHSMEGDAWgBQQDswPFQGeGMeTzPbHW62EZbbT
JzALBgkqhkiG9w0BAQUDQQAqZzN0DqUyEfR8zIanozyD2pp10m9le+ODaKZDDNfH
8cB2x26F1iZ8ccq5IC2LtQf1IKJnpTcYlLuDvW6yB96g
-----END CERTIFICATE-----
`)

	caKeyPEM = []byte(`
-----BEGIN RSA PRIVATE KEY-----
MIIBOwIBAAJBAII46mf1pYpwqvYZAa3KDAPs91817Uj0FiI8CprYjfcXn7o+oV1+
6NQq+jJMkjpcbhCj8IQJpo32yaHKr0PHMyECAwEAAQJAYctedh4raLE+Ir0a3qnK
pjQSfiUggtYTvTf7+tfAnZu946PX88ysr7XHPkXEGP4tWDTbl8BfGndrTKswVOx6
RQIhAOT5OzafJneDQ5cuGLN/hxIPBLWxKT1/25O6dhtBlRyPAiEAkZfFvCtBZyKB
JFwDdp+7gE98mXtaFrjctLWeFx797U8CIAnnqiMTwWM8H2ljyhfBtYMXeTmu3zzU
0hfS4hcNwDiLAiEAkNXXU7YEPkFJD46ps1x7/s0UOutHV8tXZD44ou+l1GkCIQDO
HOzuvYngJpoClGw0ipzJPoNZ2Z/GkdOWGByPeKu/8g==
-----END RSA PRIVATE KEY-----
`)

	nonCACert = []byte(`-----BEGIN CERTIFICATE-----
MIIBmjCCAUagAwIBAgIBADALBgkqhkiG9w0BAQUwJjENMAsGA1UEChMEanVqdTEV
MBMGA1UEAxMManVqdSB0ZXN0aW5nMB4XDTEyMTExNDE3MTU1NloXDTIyMTExNDE3
MjA1NlowJjENMAsGA1UEChMEanVqdTEVMBMGA1UEAxMManVqdSB0ZXN0aW5nMFow
CwYJKoZIhvcNAQEBA0sAMEgCQQC96/CsTTY1Va8et6QYNXwrssAi36asFlV/fksG
hqRucidiz/+xHvhs9EiqEu7NGxeVAkcfIhXu6/BDlobtj2v5AgMBAAGjYzBhMA4G
A1UdDwEB/wQEAwIABDAPBgNVHRMBAf8EBTADAgEBMB0GA1UdDgQWBBRqbxkIW4R0
vmmkUoYuWg9sDob4jzAfBgNVHSMEGDAWgBRqbxkIW4R0vmmkUoYuWg9sDob4jzAL
BgkqhkiG9w0BAQUDQQC3+KN8RppKdvlbP6fDwRC22PaCxd0PVyIHsn7I4jgpBPf8
Z3codMYYA5/f0AmUsD7wi7nnJVPPLZK7JWu4VI/w
-----END CERTIFICATE-----
`)

	nonCAKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBAL3r8KxNNjVVrx63pBg1fCuywCLfpqwWVX9+SwaGpG5yJ2LP/7Ee
+Gz0SKoS7s0bF5UCRx8iFe7r8EOWhu2Pa/kCAwEAAQJAdzuAxStUNPeuEWLJKkmp
wuVdqocuZCtBUeE/yMEOyibZ9NLKSuDJuDorkoeoiBz2vyUITHkLp4jgNmCI8NGg
AQIhAPZG9+3OghlzcqWR4nTho8KO/CuO9bu5G4jNEdIrSJ6BAiEAxWtoLZNMwI4Q
kj2moFk9GdBXZV9I0t1VTwcDvVyeAXkCIDrfvldQPdO9wJOKK3vLkS1qpyf2lhIZ
b1alx3PZuxOBAiAthPltYMRWtar+fTaZTFo5RH+SQSkibaRI534mQF+ySQIhAIml
yiWVLC2XrtwijDu1fwh/wtFCb/bPvqvgG5wgAO+2
-----END RSA PRIVATE KEY-----
`)
)
