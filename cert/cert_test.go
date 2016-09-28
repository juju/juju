// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cert_test

import (
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"testing"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cert"
)

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

type certSuite struct{}

var _ = gc.Suite(certSuite{})

func checkNotBefore(c *gc.C, cert *x509.Certificate, now time.Time) {
	// Check that the certificate is valid from one week before today.
	c.Check(cert.NotBefore.Before(now), jc.IsTrue)
	c.Check(cert.NotBefore.Before(now.AddDate(0, 0, -6)), jc.IsTrue)
	c.Check(cert.NotBefore.After(now.AddDate(0, 0, -8)), jc.IsTrue)
}

func checkNotAfter(c *gc.C, cert *x509.Certificate, expiry time.Time) {
	// Check the surrounding day.
	c.Assert(cert.NotAfter.Before(expiry.AddDate(0, 0, 1)), jc.IsTrue)
	c.Assert(cert.NotAfter.After(expiry.AddDate(0, 0, -1)), jc.IsTrue)
}

func (certSuite) TestParseCertificate(c *gc.C) {
	xcert, err := cert.ParseCert(caCertPEM)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(xcert.Subject.CommonName, gc.Equals, `juju-generated CA for model "juju testing"`)

	xcert, err = cert.ParseCert(caKeyPEM)
	c.Check(xcert, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "no certificates found")

	xcert, err = cert.ParseCert("hello")
	c.Check(xcert, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "no certificates found")
}

func (certSuite) TestParseCertAndKey(c *gc.C) {
	xcert, key, err := cert.ParseCertAndKey(caCertPEM, caKeyPEM)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(xcert.Subject.CommonName, gc.Equals, `juju-generated CA for model "juju testing"`)
	c.Assert(key, gc.NotNil)

	c.Assert(xcert.PublicKey.(*rsa.PublicKey), gc.DeepEquals, &key.PublicKey)
}

func (certSuite) TestNewCA(c *gc.C) {
	now := time.Now()
	expiry := roundTime(now.AddDate(0, 0, 1))
	caCertPEM, caKeyPEM, err := cert.NewCA("foo", expiry)
	c.Assert(err, jc.ErrorIsNil)

	caCert, caKey, err := cert.ParseCertAndKey(caCertPEM, caKeyPEM)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(caKey, gc.FitsTypeOf, (*rsa.PrivateKey)(nil))
	c.Check(caCert.Subject.CommonName, gc.Equals, `juju-generated CA for environment "foo"`)
	checkNotBefore(c, caCert, now)
	checkNotAfter(c, caCert, expiry)
	c.Check(caCert.BasicConstraintsValid, jc.IsTrue)
	c.Check(caCert.IsCA, jc.IsTrue)
	//c.Assert(caCert.MaxPathLen, Equals, 0)	TODO it ends up as -1 - check that this is ok.
}

func (certSuite) TestNewServer(c *gc.C) {
	now := time.Now()
	expiry := roundTime(now.AddDate(1, 0, 0))
	caCertPEM, caKeyPEM, err := cert.NewCA("foo", expiry)
	c.Assert(err, jc.ErrorIsNil)

	caCert, _, err := cert.ParseCertAndKey(caCertPEM, caKeyPEM)
	c.Assert(err, jc.ErrorIsNil)

	srvCertPEM, srvKeyPEM, err := cert.NewServer(caCertPEM, caKeyPEM, expiry, nil)
	c.Assert(err, jc.ErrorIsNil)
	checkCertificate(c, caCert, srvCertPEM, srvKeyPEM, now, expiry)
}

func (certSuite) TestNewDefaultServer(c *gc.C) {
	now := time.Now()
	expiry := roundTime(now.AddDate(1, 0, 0))
	caCertPEM, caKeyPEM, err := cert.NewCA("foo", expiry)
	c.Assert(err, jc.ErrorIsNil)

	caCert, _, err := cert.ParseCertAndKey(caCertPEM, caKeyPEM)
	c.Assert(err, jc.ErrorIsNil)

	srvCertPEM, srvKeyPEM, err := cert.NewDefaultServer(caCertPEM, caKeyPEM, nil)
	c.Assert(err, jc.ErrorIsNil)
	srvCertExpiry := roundTime(time.Now().AddDate(10, 0, 0))
	checkCertificate(c, caCert, srvCertPEM, srvKeyPEM, now, srvCertExpiry)
}

func checkCertificate(c *gc.C, caCert *x509.Certificate, srvCertPEM, srvKeyPEM string, now, expiry time.Time) {
	srvCert, srvKey, err := cert.ParseCertAndKey(srvCertPEM, srvKeyPEM)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(srvCert.Subject.CommonName, gc.Equals, "*")
	checkNotBefore(c, srvCert, now)
	checkNotAfter(c, srvCert, expiry)
	c.Assert(srvCert.BasicConstraintsValid, jc.IsFalse)
	c.Assert(srvCert.IsCA, jc.IsFalse)
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
		c.Assert(err, jc.ErrorIsNil)
		srvCert, _, err := cert.ParseCertAndKey(srvCertPEM, srvKeyPEM)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(srvCert.DNSNames, gc.DeepEquals, t.expectedDNSNames)
		c.Assert(srvCert.IPAddresses, gc.DeepEquals, t.expectedIPAddresses)
	}
}

func (certSuite) TestWithNonUTCExpiry(c *gc.C) {
	expiry, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", "2012-11-28 15:53:57 +0100 CET")
	c.Assert(err, jc.ErrorIsNil)
	certPEM, keyPEM, err := cert.NewCA("foo", expiry)
	xcert, err := cert.ParseCert(certPEM)
	c.Assert(err, jc.ErrorIsNil)
	checkNotAfter(c, xcert, expiry)

	var noHostnames []string
	certPEM, _, err = cert.NewServer(certPEM, keyPEM, expiry, noHostnames)
	xcert, err = cert.ParseCert(certPEM)
	c.Assert(err, jc.ErrorIsNil)
	checkNotAfter(c, xcert, expiry)
}

func (certSuite) TestNewServerWithInvalidCert(c *gc.C) {
	var noHostnames []string
	srvCert, srvKey, err := cert.NewServer(nonCACert, nonCAKey, time.Now(), noHostnames)
	c.Check(srvCert, gc.Equals, "")
	c.Check(srvKey, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, "CA certificate is not a valid CA")
}

func (certSuite) TestVerify(c *gc.C) {
	now := time.Now()
	caCert, caKey, err := cert.NewCA("foo", now.Add(1*time.Minute))
	c.Assert(err, jc.ErrorIsNil)

	var noHostnames []string
	srvCert, _, err := cert.NewServer(caCert, caKey, now.Add(3*time.Minute), noHostnames)
	c.Assert(err, jc.ErrorIsNil)

	err = cert.Verify(srvCert, caCert, now)
	c.Assert(err, jc.ErrorIsNil)

	err = cert.Verify(srvCert, caCert, now.Add(55*time.Second))
	c.Assert(err, jc.ErrorIsNil)

	err = cert.Verify(srvCert, caCert, now.AddDate(0, 0, -8))
	c.Check(err, gc.ErrorMatches, "x509: certificate has expired or is not yet valid")

	err = cert.Verify(srvCert, caCert, now.Add(2*time.Minute))
	c.Check(err, gc.ErrorMatches, "x509: certificate has expired or is not yet valid")

	caCert2, caKey2, err := cert.NewCA("bar", now.Add(1*time.Minute))
	c.Assert(err, jc.ErrorIsNil)

	// Check original server certificate against wrong CA.
	err = cert.Verify(srvCert, caCert2, now)
	c.Check(err, gc.ErrorMatches, "x509: certificate signed by unknown authority")

	srvCert2, _, err := cert.NewServer(caCert2, caKey2, now.Add(1*time.Minute), noHostnames)
	c.Assert(err, jc.ErrorIsNil)

	// Check new server certificate against original CA.
	err = cert.Verify(srvCert2, caCert, now)
	c.Check(err, gc.ErrorMatches, "x509: certificate signed by unknown authority")
}

// checkTLSConnection checks that we can correctly perform a TLS
// handshake using the given credentials.
func checkTLSConnection(c *gc.C, caCert, srvCert *x509.Certificate, srvKey *rsa.PrivateKey) (caName string) {
	clientCertPool := x509.NewCertPool()
	clientCertPool.AddCert(caCert)

	var outBytes bytes.Buffer

	const msg = "hello to the server"
	p0, p1 := net.Pipe()
	p0 = &recordingConn{
		Conn:   p0,
		Writer: io.MultiWriter(p0, &outBytes),
	}

	var clientState tls.ConnectionState
	done := make(chan error)
	go func() {
		config := utils.SecureTLSConfig()

		config.Certificates = []tls.Certificate{{
			Certificate: [][]byte{srvCert.Raw},
			PrivateKey:  srvKey,
		}}

		conn := tls.Server(p1, config)
		defer conn.Close()
		data, err := ioutil.ReadAll(conn)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(data), gc.Equals, msg)
		close(done)
	}()

	cfg := utils.SecureTLSConfig()
	cfg.ServerName = "anyServer"
	cfg.RootCAs = clientCertPool
	clientConn := tls.Client(p0, cfg)
	defer clientConn.Close()

	_, err := clientConn.Write([]byte(msg))
	c.Assert(err, jc.ErrorIsNil)
	clientState = clientConn.ConnectionState()
	clientConn.Close()

	// wait for server to exit
	<-done

	outData := outBytes.String()
	c.Assert(outData, gc.Not(gc.HasLen), 0)
	if strings.Index(outData, msg) != -1 {
		c.Fatalf("TLS connection not encrypted")
	}
	c.Assert(clientState.VerifiedChains, gc.HasLen, 1)
	c.Assert(clientState.VerifiedChains[0], gc.HasLen, 2)
	return clientState.VerifiedChains[0][1].Subject.CommonName
}

type recordingConn struct {
	net.Conn
	io.Writer
}

func (c recordingConn) Write(buf []byte) (int, error) {
	return c.Writer.Write(buf)
}

// roundTime returns t rounded to the previous whole second.
func roundTime(t time.Time) time.Time {
	return t.Add(time.Duration(-t.Nanosecond()))
}

var (
	caCertPEM = `
-----BEGIN CERTIFICATE-----
MIICHDCCAcagAwIBAgIUfzWn5ktGMxD6OiTgfiZyvKdM+ZYwDQYJKoZIhvcNAQEL
BQAwazENMAsGA1UEChMEanVqdTEzMDEGA1UEAwwqanVqdS1nZW5lcmF0ZWQgQ0Eg
Zm9yIG1vZGVsICJqdWp1IHRlc3RpbmciMSUwIwYDVQQFExwxMjM0LUFCQ0QtSVMt
Tk9ULUEtUkVBTC1VVUlEMB4XDTE2MDkyMTEwNDgyN1oXDTI2MDkyODEwNDgyN1ow
azENMAsGA1UEChMEanVqdTEzMDEGA1UEAwwqanVqdS1nZW5lcmF0ZWQgQ0EgZm9y
IG1vZGVsICJqdWp1IHRlc3RpbmciMSUwIwYDVQQFExwxMjM0LUFCQ0QtSVMtTk9U
LUEtUkVBTC1VVUlEMFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBAL+0X+1zl2vt1wI4
1Q+RnlltJyaJmtwCbHRhREXVGU7t0kTMMNERxqLnuNUyWRz90Rg8s9XvOtCqNYW7
mypGrFECAwEAAaNCMEAwDgYDVR0PAQH/BAQDAgKkMA8GA1UdEwEB/wQFMAMBAf8w
HQYDVR0OBBYEFHueMLZ1QJ/2sKiPIJ28TzjIMRENMA0GCSqGSIb3DQEBCwUAA0EA
ovZN0RbUHrO8q9Eazh0qPO4mwW9jbGTDz126uNrLoz1g3TyWxIas1wRJ8IbCgxLy
XUrBZO5UPZab66lJWXyseA==
-----END CERTIFICATE-----
`

	caKeyPEM = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBAL+0X+1zl2vt1wI41Q+RnlltJyaJmtwCbHRhREXVGU7t0kTMMNER
xqLnuNUyWRz90Rg8s9XvOtCqNYW7mypGrFECAwEAAQJAMPa+JaUHgO6foxam/LIB
0u95N3OgFR+dWeBaEsgKDclpREdJ0rXNI+3C3kwqeEZR4omoPlBeSEewSkwHxpmI
0QIhAOjKiHZ5v6R8haleipbDzkGUnZW07hEwL5Ld4MNx/QQ1AiEA0tEzSSNAdM0C
M/vY0x5mekIYai8/tFSEG9PJ3ZkpEy0CIQCo9B3YxwI1Un777vbs903iQQeiWP+U
EAHnOQvhLgDxpQIgGkpml+9igW5zoOH+h02aQBLwEoXz7tw/YW0HFrCcE70CIGkS
ve4WjiEqnQaHNAPy0hY/1DfIgBOSpOfnkFHOk9vX
-----END RSA PRIVATE KEY-----
`

	nonCACert = `
-----BEGIN CERTIFICATE-----
MIIB8jCCAZygAwIBAgIVANueMZWTFEIx6AcNAWsG4VL4sUn5MA0GCSqGSIb3DQEB
CwUAMGsxDTALBgNVBAoTBGp1anUxMzAxBgNVBAMMKmp1anUtZ2VuZXJhdGVkIENB
IGZvciBtb2RlbCAianVqdSB0ZXN0aW5nIjElMCMGA1UEBRMcMTIzNC1BQkNELUlT
LU5PVC1BLVJFQUwtVVVJRDAeFw0xNjA5MjExMDQ4MjdaFw0yNjA5MjgxMDQ4Mjda
MBsxDTALBgNVBAoTBGp1anUxCjAIBgNVBAMTASowXDANBgkqhkiG9w0BAQEFAANL
ADBIAkEAwZps3qpPu2FCAhbxolf/BvSa+dMal3AhPMe+lwTuSbtS81W+WSrbwUSI
ZKSGHYDpFRN6ytNjt1oPbDNKDIR30wIDAQABo2cwZTAOBgNVHQ8BAf8EBAMCA6gw
EwYDVR0lBAwwCgYIKwYBBQUHAwEwHQYDVR0OBBYEFNNUDrcyP/4RbGBpKeC3gmfL
kjlwMB8GA1UdIwQYMBaAFHueMLZ1QJ/2sKiPIJ28TzjIMRENMA0GCSqGSIb3DQEB
CwUAA0EALiurKx//Qh5TQQ0TmT0P5f7OFLIs5XPSS98Lseb92h12CPNO4kB000Yh
Xa7kZRGngwFbvjzqZ0eOfmo0l8M23A==
-----END CERTIFICATE-----
`

	nonCAKey = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOwIBAAJBAMGabN6qT7thQgIW8aJX/wb0mvnTGpdwITzHvpcE7km7UvNVvlkq
28FEiGSkhh2A6RUTesrTY7daD2wzSgyEd9MCAwEAAQJBAKfeuOvRjVUSneOl9Vsp
Je7oBcD9dR8+kPNc1zungN7YVhIuxqvzXJSPeMGsHloPI+BcFFXv3t+eVCDT9sPL
L+ECIQDq1nqVIEX3k5nn6eI0L5CQbIfEyvWGJ/mOGSo9TWdN+QIhANMMsopPb9ct
Z61LqPmTtNX4nhHyMEjxbUzqzsZzsRcrAiBeYyhP6fHVSXERopK1kOyU79o+Aalf
a4/FSl4M16CO2QIgOBQZpNKyvxRbhhqijZ6H4IstRUt7NQahqlyCEQ1Qsv0CIQDQ
tUzgFwUpd6NVButkqWGqnmBeKUOs97dqSyOzN9Nk8w==
-----END RSA PRIVATE KEY-----
`
)
