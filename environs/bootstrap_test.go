package environs_test

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
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
	"net"
	"os"
	"path/filepath"
	"strings"
)

type bootstrapSuite struct {
	oldHome string
	testing.LoggingSuite
}

var _ = Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.oldHome = os.Getenv("HOME")
	home := c.MkDir()
	os.Setenv("HOME", home)
	err := os.Mkdir(filepath.Join(home, ".juju"), 0777)
	c.Assert(err, IsNil)
}

func (s *bootstrapSuite) TearDownTest(c *C) {
	os.Setenv("HOME", s.oldHome)
}

func (s *bootstrapSuite) TestBootstrapKeyGeneration(c *C) {
	env := newEnviron("foo", nil, nil)
	err := environs.Bootstrap(env, false, nil)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	bootstrapCert, bootstrapKey := parseCertAndKey(c, env.certPEM, env.keyPEM)

	// Check that the generated CA key has been written correctly.
	caCertPEM, err := ioutil.ReadFile(filepath.Join(os.Getenv("HOME"), ".juju", "foo-cert.pem"))
	c.Assert(err, IsNil)
	caKeyPEM, err := ioutil.ReadFile(filepath.Join(os.Getenv("HOME"), ".juju", "foo-private-key.pem"))
	c.Assert(err, IsNil)

	// Check that the cert and key have been set correctly in the configuration
	cfgCertPEM, cfgCertOK := env.cfg.CACertPEM()
	cfgKeyPEM, cfgKeyOK := env.cfg.CAPrivateKeyPEM()
	c.Assert(cfgCertOK, Equals, true)
	c.Assert(cfgKeyOK, Equals, true)
	c.Assert(cfgCertPEM, DeepEquals, caCertPEM)
	c.Assert(cfgKeyPEM, DeepEquals, caKeyPEM)

	caCert, _ := parseCertAndKey(c, caCertPEM, caKeyPEM)

	caName := checkTLSConnection(c, caCert, bootstrapCert, bootstrapKey)
	c.Assert(caName, Equals, `juju-generated CA for environment foo`)
}

func (s *bootstrapSuite) TestBootstrapFuncKeyGeneration(c *C) {
	env := newEnviron("foo", nil, nil)
	saved := make(map[string][]byte)
	err := environs.Bootstrap(env, false, func(name string, data []byte) error {
		saved[name] = data
		return nil
	})
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	bootstrapCert, bootstrapKey := parseCertAndKey(c, env.certPEM, env.keyPEM)

	// Check that the cert and key have been set correctly in the configuration
	cfgCertPEM, cfgCertOK := env.cfg.CACertPEM()
	cfgKeyPEM, cfgKeyOK := env.cfg.CAPrivateKeyPEM()
	c.Assert(cfgCertOK, Equals, true)
	c.Assert(cfgKeyOK, Equals, true)
	c.Assert(cfgCertPEM, DeepEquals, saved["foo-cert.pem"])
	c.Assert(cfgKeyPEM, DeepEquals, saved["foo-private-key.pem"])
	c.Assert(saved, HasLen, 2)

	caCert, _ := parseCertAndKey(c, cfgCertPEM, cfgKeyPEM)

	caName := checkTLSConnection(c, caCert, bootstrapCert, bootstrapKey)
	c.Assert(caName, Equals, `juju-generated CA for environment foo`)
}

func panicWrite(name string, data []byte) error {
	panic("writeCertFile called unexpectedly")
}

func (s *bootstrapSuite) TestBootstrapExistingKey(c *C) {
	env := newEnviron("foo", []byte(testing.CACertPEM), []byte(testing.CAKeyPEM))
	err := environs.Bootstrap(env, false, panicWrite)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)

	bootstrapCert, bootstrapKey := parseCertAndKey(c, env.certPEM, env.keyPEM)

	caName := checkTLSConnection(c, testing.CACertX509, bootstrapCert, bootstrapKey)
	c.Assert(caName, Equals, testing.CACertX509.Subject.CommonName)
}

func (s *bootstrapSuite) TestBootstrapUploadTools(c *C) {
	env := newEnviron("foo", nil, nil)
	err := environs.Bootstrap(env, false, nil)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	c.Assert(env.uploadTools, Equals, false)

	env = newEnviron("foo", nil, nil)
	err = environs.Bootstrap(env, true, nil)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	c.Assert(env.uploadTools, Equals, true)
}

var (
	nonCACert = `-----BEGIN CERTIFICATE-----
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
`
	nonCAKey = `-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBAL3r8KxNNjVVrx63pBg1fCuywCLfpqwWVX9+SwaGpG5yJ2LP/7Ee
+Gz0SKoS7s0bF5UCRx8iFe7r8EOWhu2Pa/kCAwEAAQJAdzuAxStUNPeuEWLJKkmp
wuVdqocuZCtBUeE/yMEOyibZ9NLKSuDJuDorkoeoiBz2vyUITHkLp4jgNmCI8NGg
AQIhAPZG9+3OghlzcqWR4nTho8KO/CuO9bu5G4jNEdIrSJ6BAiEAxWtoLZNMwI4Q
kj2moFk9GdBXZV9I0t1VTwcDvVyeAXkCIDrfvldQPdO9wJOKK3vLkS1qpyf2lhIZ
b1alx3PZuxOBAiAthPltYMRWtar+fTaZTFo5RH+SQSkibaRI534mQF+ySQIhAIml
yiWVLC2XrtwijDu1fwh/wtFCb/bPvqvgG5wgAO+2
-----END RSA PRIVATE KEY-----
`
)

func (s *bootstrapSuite) TestBootstrapWithInvalidCert(c *C) {
	env := newEnviron("foo", []byte(nonCACert), []byte(nonCAKey))
	err := environs.Bootstrap(env, false, panicWrite)
	c.Assert(err, ErrorMatches, "cannot generate bootstrap certificate: CA certificate is not a valid CA")
}

// checkTLSConnection checks that we can correctly perform a TLS
// handshake using the given credentials.
func checkTLSConnection(c *C, caCert, bootstrapCert *x509.Certificate, bootstrapKey *rsa.PrivateKey) (caName string) {
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

type bootstrapEnviron struct {
	name             string
	cfg              *config.Config
	environs.Environ // stub out all methods we don't care about.

	// The following fields are filled in when Bootstrap is called.
	bootstrapCount int
	uploadTools    bool
	certPEM        []byte
	keyPEM         []byte
}

func newEnviron(name string, caCertPEM, caKeyPEM []byte) *bootstrapEnviron {
	m := map[string]interface{}{
		"name":            name,
		"type":            "test",
		"authorized-keys": "foo",
		"ca-cert":         nil,
		"ca-private-key":  nil,
	}
	if caCertPEM != nil {
		m["ca-cert"] = string(caCertPEM)
	}
	if caKeyPEM != nil {
		m["ca-private-key"] = string(caKeyPEM)
	}
	cfg, err := config.New(m)
	if err != nil {
		panic(fmt.Errorf("cannot create config from %#v: %v", m, err))
	}
	return &bootstrapEnviron{
		name: name,
		cfg:  cfg,
	}
}

func (e *bootstrapEnviron) Name() string {
	return e.name
}

func (e *bootstrapEnviron) Bootstrap(uploadTools bool, certPEM, keyPEM []byte) error {
	e.bootstrapCount++
	e.uploadTools = uploadTools
	e.certPEM = certPEM
	e.keyPEM = keyPEM
	return nil
}

func (e *bootstrapEnviron) Config() *config.Config {
	return e.cfg
}

func (e *bootstrapEnviron) SetConfig(cfg *config.Config) error {
	e.cfg = cfg
	return nil
}

func x509ToPEM(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
}

func parseCertAndKey(c *C, certPEM, keyPEM []byte) (cert *x509.Certificate, key *rsa.PrivateKey) {
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	c.Assert(err, IsNil)

	cert, err = x509.ParseCertificate(tlsCert.Certificate[0])
	c.Assert(err, IsNil)

	return cert, tlsCert.PrivateKey.(*rsa.PrivateKey)
}
