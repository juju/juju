package cert_test

import (
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cert"
	"net"
	"strings"
	"testing"
	"time"
)

func TestAll(t *testing.T) {
	TestingT(t)
}

type certSuite struct{}

var _ = Suite(certSuite{})

func (certSuite) TestParseCertificate(c *C) {
	xcert, err := cert.ParseCertificate(certPEM)
	c.Assert(err, IsNil)
	c.Assert(xcert.Subject.CommonName, Equals, "juju testing")

	xcert, err = cert.ParseCertificate(keyPEM)
	c.Check(xcert, IsNil)
	c.Assert(err, ErrorMatches, "no certificates found")

	xcert, err = cert.ParseCertificate([]byte("hello"))
	c.Check(xcert, IsNil)
	c.Assert(err, ErrorMatches, "no certificates found")
}

func (certSuite) TestParseCertAndKey(c *C) {
	xcert, key, err := cert.ParseCertAndKey(certPEM, keyPEM)
	c.Assert(err, IsNil)
	c.Assert(xcert.Subject.CommonName, Equals, "juju testing")
	c.Assert(key, NotNil)

	c.Assert(xcert.PublicKey.(*rsa.PublicKey), DeepEquals, &key.PublicKey)
}

var certPEM = []byte(`
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

var keyPEM = []byte(`
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

func (certSuite) TestNewCA(c *C) {
	expiry := roundTime(time.Now().AddDate(0, 0, 1))
	caCertPEM, caKeyPEM, err := cert.NewCA("foo", expiry)
	c.Assert(err, IsNil)

	caCert, caKey, err := cert.ParseCertAndKey(caCertPEM, caKeyPEM)
	c.Assert(err, IsNil)

	c.Assert(caKey, FitsTypeOf, (*rsa.PrivateKey)(nil))
	c.Assert(caCert.Subject.CommonName, Equals, "juju-generated CA for environment foo")
	c.Assert(caCert.NotAfter.Equal(expiry), Equals, true)
	c.Assert(caCert.BasicConstraintsValid, Equals, true)
	c.Assert(caCert.IsCA, Equals, true)
	//c.Assert(caCert.MaxPathLen, Equals, 0)	TODO it ends up as -1 - check that this is ok.
}

func (certSuite) TestNewServer(c *C) {
	expiry := roundTime(time.Now().AddDate(1, 0, 0))
	caCertPEM, caKeyPEM, err := cert.NewCA("foo", expiry)
	c.Assert(err, IsNil)

	caCert, _, err := cert.ParseCertAndKey(caCertPEM, caKeyPEM)
	c.Assert(err, IsNil)

	srvCertPEM, srvKeyPEM, err := cert.NewServer("juju test", caCertPEM, caKeyPEM, expiry)
	c.Assert(err, IsNil)

	srvCert, srvKey, err := cert.ParseCertAndKey(srvCertPEM, srvKeyPEM)
	c.Assert(err, IsNil)
	c.Assert(err, IsNil)
	c.Assert(srvCert.Subject.CommonName, Equals, "*")
	c.Assert(srvCert.NotAfter.Equal(expiry), Equals, true)
	c.Assert(srvCert.BasicConstraintsValid, Equals, false)
	c.Assert(srvCert.IsCA, Equals, false)

	checkTLSConnection(c, caCert, srvCert, srvKey)
}

// checkTLSConnection checks that we can correctly perform a TLS
// handshake using the given credentials.
func checkTLSConnection(c *C, caCert, srvCert *x509.Certificate, srvKey *rsa.PrivateKey) (caName string) {
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
		c.Check(err, IsNil)
	}

	outData := string(outBytes.Bytes())
	c.Assert(outData, Not(HasLen), 0)
	if strings.Index(outData, msg) != -1 {
		c.Fatalf("TLS connection not encrypted")
	}
	c.Assert(clientState.VerifiedChains, HasLen, 1)
	c.Assert(clientState.VerifiedChains[0], HasLen, 2)
	return clientState.VerifiedChains[0][1].Subject.CommonName
}

func newTLSCert(c *C, cert *x509.Certificate, key *rsa.PrivateKey) tls.Certificate {
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

// recordongConn returns a connection which
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
