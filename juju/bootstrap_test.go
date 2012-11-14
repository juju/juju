package juju_test
import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju"
)

type bootstrapSuite struct{}

var _ = Suite(&bootstrapSuite{})

// without private key
// with invalid keys

// home directory - already existing; generated.

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
	c.Assert(env.uploadTools, Equals, false)

	bootstrapCert, bootstrapKey := parseCertAndKey(env.certAndKey)

	// Check that the generated root key has been written
	// correctly.
	rootKeyPEM, err := ioutil.ReadFile(filepath.Join(home, ".juju", "foo-cert.pem"))
	c.Assert(err, IsNil)

	rootCert, rootKey := parseCertAndKey(rootKeyPEM)
	
	checkTLSConnection(c, rootCert, bootstrapCert, bootstrapKey)
}

// checkTLSConnection checks that we can correctly perform
// a TLS handshake using the given credentials.
func checkTLSConnection(c *C, rootCert, bootstrapCert *x509.Certificate, bootstrapKey *rsa.PrivateKey) {
	clientCertPool := x509.NewCertPool()
	clientCertPool.AddCert(rootCert)

	var inBytes, outBytes bytes.Buffer

	const msg = "hello to the server"
	p0, p1 := net.Pipe()
	p0 = bufferedConn(p0, 3)
	p0 = recordingConn(p0, &inBytes, &outBytes)

	done := make(chan error)
	go func() {
		clientConn := tls.Client(p0, &tls.Config{
			ServerName: "any",
			RootCAs:    clientCertPool,
		})
		defer clientConn.Close()

		_, err := clientConn.Write([]byte(msg))
		if err != nil {
			done <- fmt.Errorf("client: %v", err)
		}
		done <- nil
	}()
	go func() {
		serverConn := tls.Server(p1, &tls.Config{})
		defer serverConn.Close()
		data, err := io.ReadAll(serverConn)
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
		c.Assert(err, IsNil)
	}

	outData := string(outBytes.Bytes())
	c.Assert(outData, Not(HasLen), 0)
	if strings.Index(outData, msg) != -1 {
		c.Fatalf("TLS connection not encrypted")
	}
}

// bufferedConn adds buffering for at least
// n writes to either end of the given connection.
func bufferedConn(c net.Conn, n int) net.Conn {
	for i := 0; i < n; i++ {
		p0, p1 := net.Pipe()
		go copyClose(c, p0)
		go copyClose(p1, c)
		c = p0
	}
	return c
}

// recordongConn returns a connection which
// records traffic in or out of the given connection.
func recordingConn(c net.Conn, in, out io.Writer) net.Conn {
	p0, p1 := net.Pipe()
	go func() {
		io.Copy(io.MultiWriter(c, out), p0)
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

type bootstapEnviron struct {
	name string
	bootstrapCount int
	uploadTools bool
	certAndKey []byte
	environs.Environ
}

func (e *bootstrapEnviron) Name() string {
	return e.name
}

func (e *bootstrapEnviron) Bootstrap(uploadTools bool, certAndKey []byte) error {
	e.bootstrapCount++
	e.uploadTools = uploadTools
	e.certAndKey = certAndKey
	return nil
}

func parseCertAndKey(c *C, certAndKey []byte) (cert *x509.Certificate, key *rsa.PrivateKey) {
	var certBlocks, otherBlocks []*pem.Block
	for {
		var b *pem.Block
		b, certAndKey = pem.Decode(certAndKey)
		if b == nil {
			break
		}
		if b.Type == "CERTIFICATE" {
			certBlocks = append(certBlocks, b)
		} else {
			otherBlocks = append(otherBlocks, b)
		}
	}
	c.Assert(certBlocks, HasLen, 1)
	c.Assert(otherBlocks, HasLen, 1)
	tlsCert, err := tls.X509KeyPair(pem.EncodeToMemory(certBlock), pem.EncodeToMemory(keyBlock))
	c.Assert(err, IsNil)
	cert, err = x509.ParseCertificate(tlsCert.Certificate[0])
	c.Assert(err, IsNil)
	key = tlsCert.PrivateKey.(*rsa.PrivateKey)
	return
}
