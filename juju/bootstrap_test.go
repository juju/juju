package juju_test

import (
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/testing"
	"net"
	"os"
	"path/filepath"
	"strings"
)

type bootstrapSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestBootstrapKeyGeneration(c *C) {
	defer os.Setenv("HOME", os.Getenv("HOME"))
	home := c.MkDir()
	os.Setenv("HOME", home)
	err := os.Mkdir(filepath.Join(home, ".juju"), 0777)
	c.Assert(err, IsNil)

	env := &bootstrapEnviron{name: "foo"}
	err = juju.Bootstrap(env, false, nil)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)

	bootstrapCert, bootstrapKey := parseCertAndKey(c, env.stateServerPEM)

	// Check that the generated root key has been written
	// correctly.
	rootKeyPEM, err := ioutil.ReadFile(filepath.Join(home, ".juju", "foo-cert.pem"))
	c.Assert(err, IsNil)

	rootCert, _ := parseCertAndKey(c, rootKeyPEM)

	rootName := checkTLSConnection(c, rootCert, bootstrapCert, bootstrapKey)
	c.Assert(rootName, Equals, "juju-generated root CA for environment foo")
}

var testServerPEM = []byte(testing.RootCertPEM + testing.RootKeyPEM)

func (s *bootstrapSuite) TestBootstrapExistingKey(c *C) {
	defer os.Setenv("HOME", os.Getenv("HOME"))
	home := c.MkDir()
	os.Setenv("HOME", home)
	err := os.Mkdir(filepath.Join(home, ".juju"), 0777)
	c.Assert(err, IsNil)

	path := filepath.Join(home, ".juju", "bar-cert.pem")
	err = ioutil.WriteFile(path, testServerPEM, 0600)
	c.Assert(err, IsNil)

	env := &bootstrapEnviron{name: "bar"}
	err = juju.Bootstrap(env, false, nil)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)

	bootstrapCert, bootstrapKey := parseCertAndKey(c, env.stateServerPEM)

	rootName := checkTLSConnection(c, certificate(testing.RootCertPEM), bootstrapCert, bootstrapKey)
	c.Assert(rootName, Equals, testing.RootCertX509.Subject.CommonName)
}

func (s *bootstrapSuite) TestBootstrapUploadTools(c *C) {
	env := &bootstrapEnviron{name: "foo"}
	err := juju.Bootstrap(env, false, testServerPEM)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	c.Assert(env.uploadTools, Equals, false)

	env = &bootstrapEnviron{name: "foo"}
	err = juju.Bootstrap(env, true, testServerPEM)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	c.Assert(env.uploadTools, Equals, true)
}

func (s *bootstrapSuite) TestBootstrapWithCertArgument(c *C) {
	env := &bootstrapEnviron{name: "bar"}
	err := juju.Bootstrap(env, false, testServerPEM)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)

	bootstrapCert, bootstrapKey := parseCertAndKey(c, env.stateServerPEM)

	rootName := checkTLSConnection(c, certificate(testing.RootCertPEM), bootstrapCert, bootstrapKey)
	c.Assert(rootName, Equals, testing.RootCertX509.Subject.CommonName)
}

var invalidCertTests = []struct {
	pem string
	err string
}{{
	`xxxx`,
	"bad root CA PEM: root PEM holds no private key",
}, {
	testing.RootCertPEM,
	"bad root CA PEM: root PEM holds no private key",
}, {
	testing.RootKeyPEM,
	"bad root CA PEM: root PEM holds no certificate",
}, {
	`-----BEGIN CERTIFICATE-----
MIIBnTCCAUmgAwIBAgIBADALBgkqhkiG9w0BAQUwJjENMAsGA1UEChMEanVqdTEV
MBMGA1UEAxMManVqdSB0ZXN0aW5nMB4XDTEyMTExNDE0Mzg1NFoXDTIyMTExNDE0
NDM1NFowJjENMAsGA1UEChMEanVqdTEVMBMGA1UEAxMManVqdSB0ZXN0aW5n
-----END CERTIFICATE-----
` + testing.RootKeyPEM,
	`bad root CA PEM: ASN\.1.*`,
}, {
	`-----BEGIN RSA PRIVATE KEY-----
MIIBOwIBAAJBAII46mf1pYpwqvYZAa3KDAPs91817Uj0FiI8CprYjfcXn7o+oV1+
-----END RSA PRIVATE KEY-----
` + testing.RootCertPEM,
	"bad root CA PEM: crypto/tls: failed to parse key: .*",
}, {
	`-----BEGIN CERTIFICATE-----
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

-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBAL3r8KxNNjVVrx63pBg1fCuywCLfpqwWVX9+SwaGpG5yJ2LP/7Ee
+Gz0SKoS7s0bF5UCRx8iFe7r8EOWhu2Pa/kCAwEAAQJAdzuAxStUNPeuEWLJKkmp
wuVdqocuZCtBUeE/yMEOyibZ9NLKSuDJuDorkoeoiBz2vyUITHkLp4jgNmCI8NGg
AQIhAPZG9+3OghlzcqWR4nTho8KO/CuO9bu5G4jNEdIrSJ6BAiEAxWtoLZNMwI4Q
kj2moFk9GdBXZV9I0t1VTwcDvVyeAXkCIDrfvldQPdO9wJOKK3vLkS1qpyf2lhIZ
b1alx3PZuxOBAiAthPltYMRWtar+fTaZTFo5RH+SQSkibaRI534mQF+ySQIhAIml
yiWVLC2XrtwijDu1fwh/wtFCb/bPvqvgG5wgAO+2
-----END RSA PRIVATE KEY-----
`, "bad root CA PEM: root certificate is not a valid CA",
}}

func (s *bootstrapSuite) TestBootstrapWithInvalidCert(c *C) {
	for i, test := range invalidCertTests {
		c.Logf("test %d", i)
		env := &bootstrapEnviron{name: "foo"}
		err := juju.Bootstrap(env, false, []byte(test.pem))
		c.Check(env.bootstrapCount, Equals, 0)
		c.Assert(err, ErrorMatches, test.err)
	}
}

// checkTLSConnection checks that we can correctly perform a TLS
// handshake using the given credentials.
func checkTLSConnection(c *C, rootCert, bootstrapCert certificate, bootstrapKey *rsa.PrivateKey) (rootCAName string) {
	clientCertPool := x509.NewCertPool()
	clientCertPool.AddCert(rootCert.x509(c))

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
				newTLSCert(c, bootstrapCert, bootstrapKey),
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

func newTLSCert(c *C, cert certificate, key *rsa.PrivateKey) tls.Certificate {
	return tls.Certificate{
		Certificate: [][]byte{cert.der(c)},
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

type bootstrapEnviron struct {
	name           string
	bootstrapCount int
	uploadTools    bool
	stateServerPEM []byte
	environs.Environ
}

func (e *bootstrapEnviron) Name() string {
	return e.name
}

func (e *bootstrapEnviron) Bootstrap(uploadTools bool, stateServerPEM []byte) error {
	e.bootstrapCount++
	e.uploadTools = uploadTools
	e.stateServerPEM = stateServerPEM
	return nil
}

// certificate holds a certificate in PEM format.
type certificate []byte

func (cert certificate) x509(c *C) (x509Cert *x509.Certificate) {
	for _, b := range decodePEMBlocks(cert) {
		if b.Type != "CERTIFICATE" {
			continue
		}
		if x509Cert != nil {
			c.Errorf("found extra certificate")
			continue
		}
		var err error
		x509Cert, err = x509.ParseCertificate(b.Bytes)
		c.Assert(err, IsNil)
	}
	return
}

func (cert certificate) der(c *C) []byte {
	for _, b := range decodePEMBlocks(cert) {
		if b.Type != "CERTIFICATE" {
			continue
		}
		return b.Bytes
	}
	c.Fatalf("no certificate found in cert PEM")
	panic("unreachable")
}

func decodePEMBlocks(pemData []byte) (blocks []*pem.Block) {
	for {
		var b *pem.Block
		b, pemData = pem.Decode(pemData)
		if b == nil {
			break
		}
		blocks = append(blocks, b)
	}
	return
}

func parseCertAndKey(c *C, stateServerPEM []byte) (cert certificate, key *rsa.PrivateKey) {
	var certBlocks, otherBlocks []*pem.Block
	for _, b := range decodePEMBlocks(stateServerPEM) {
		if b.Type == "CERTIFICATE" {
			certBlocks = append(certBlocks, b)
		} else {
			otherBlocks = append(otherBlocks, b)
		}
	}
	c.Assert(certBlocks, HasLen, 1)
	c.Assert(otherBlocks, HasLen, 1)
	cert = certificate(pem.EncodeToMemory(certBlocks[0]))
	tlsCert, err := tls.X509KeyPair(cert, pem.EncodeToMemory(otherBlocks[0]))
	c.Assert(err, IsNil)

	return cert, tlsCert.PrivateKey.(*rsa.PrivateKey)
}
