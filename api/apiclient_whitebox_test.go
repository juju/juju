// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	proxyutils "github.com/juju/proxy"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jtesting "github.com/juju/juju/testing"
	"github.com/juju/juju/utils/proxy"
)

type apiclientWhiteboxSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&apiclientWhiteboxSuite{})

func (s *apiclientWhiteboxSuite) TestDialWebsocketMultiCancelled(c *gc.C) {
	ctx := context.TODO()
	ctx, cancel := context.WithCancel(ctx)
	started := make(chan struct{})
	go func() {
		select {
		case <-started:
		case <-time.After(jtesting.LongWait):
			c.Fatalf("timed out waiting %s for started", jtesting.LongWait)
		}
		<-time.After(10 * time.Millisecond)
		if cancel != nil {
			c.Logf("cancelling")
			cancel()
		}
	}()
	listen, err := net.Listen("tcp4", ":0")
	c.Assert(err, jc.ErrorIsNil)
	addr := listen.Addr().String()
	c.Logf("listening at: %s", addr)
	// Note that we Listen, but we never Accept
	close(started)
	info := &Info{
		Addrs: []string{addr},
	}
	opts := DialOpts{
		DialAddressInterval: 50 * time.Millisecond,
		RetryDelay:          40 * time.Millisecond,
		Timeout:             100 * time.Millisecond,
		DialTimeout:         100 * time.Millisecond,
	}
	// Close before we connect
	listen.Close()
	_, err = dialAPI(ctx, info, opts)
	c.Check(err, gc.NotNil)
}

func (s *apiclientWhiteboxSuite) TestDialWebsocketMultiClosed(c *gc.C) {
	listen, err := net.Listen("tcp4", ":0")
	c.Assert(err, jc.ErrorIsNil)
	addr := listen.Addr().String()
	c.Logf("listening at: %s", addr)
	// Note that we Listen, but we never Accept
	info := &Info{
		Addrs: []string{addr},
	}
	opts := DialOpts{
		DialAddressInterval: 1 * time.Second,
		RetryDelay:          1 * time.Second,
		Timeout:             2 * time.Second,
		DialTimeout:         3 * time.Second,
	}
	listen.Close()
	_, _, err = DialAPI(info, opts)
	c.Check(err, gc.ErrorMatches, fmt.Sprintf("unable to connect to API: dial tcp %s:.*", regexp.QuoteMeta(addr)))
}

func (s *apiclientWhiteboxSuite) TestProxyForRequestNormalizesWebsocketSchemes(c *gc.C) {
	tests := []struct {
		about    string
		settings proxyutils.Settings
		rawURL   string
		expected string
	}{
		{
			about: "wss uses https proxy",
			settings: proxyutils.Settings{
				Https: "https://proxy.example:8443",
			},
			rawURL:   "wss://controller.example:17070/model/uuid/api",
			expected: "https://proxy.example:8443",
		},
		{
			about: "ws uses http proxy",
			settings: proxyutils.Settings{
				Http: "http://proxy.example:8080",
			},
			rawURL:   "ws://controller.example:17070/model/uuid/api",
			expected: "http://proxy.example:8080",
		},
		{
			about: "wss honours no_proxy",
			settings: proxyutils.Settings{
				Https:   "https://proxy.example:8443",
				NoProxy: "controller.example",
			},
			rawURL:   "wss://controller.example:17070/model/uuid/api",
			expected: "",
		},
	}

	for _, test := range tests {
		c.Logf("test: %s", test.about)
		err := proxy.DefaultConfig.Set(test.settings)
		c.Assert(err, jc.ErrorIsNil)

		target, err := url.Parse(test.rawURL)
		c.Assert(err, jc.ErrorIsNil)

		proxyURL, err := proxyForRequest(&http.Request{URL: target})
		c.Assert(err, jc.ErrorIsNil)
		if test.expected == "" {
			c.Assert(proxyURL, gc.IsNil)
		} else {
			c.Assert(proxyURL, gc.NotNil)
			c.Assert(proxyURL.String(), gc.Equals, test.expected)
		}
	}

	c.Assert(proxy.DefaultConfig.Set(proxyutils.Settings{}), jc.ErrorIsNil)
}

func (s *apiclientWhiteboxSuite) TestNewPrimaryHTTPTransportUsesProxyConfig(c *gc.C) {
	err := proxy.DefaultConfig.Set(proxyutils.Settings{
		Https: "https://proxy.example:8443",
	})
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Assert(proxy.DefaultConfig.Set(proxyutils.Settings{}), jc.ErrorIsNil)
	}()

	transport := newPrimaryHTTPTransport(nil)
	c.Assert(transport.Proxy, gc.NotNil)
	c.Assert(transport.MaxIdleConns, gc.Equals, 1)
	c.Assert(transport.IdleConnTimeout, gc.Equals, 90*time.Second)

	req, err := http.NewRequest(http.MethodGet, "https://controller.example:17070/model/uuid", nil)
	c.Assert(err, jc.ErrorIsNil)

	proxyURL, err := transport.Proxy(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(proxyURL, gc.NotNil)
	c.Assert(proxyURL.String(), gc.Equals, "https://proxy.example:8443")
}

// newTestCert creates a certificate signed by parent (or self-signed when
// parent is nil), returning the parsed cert and its private key.
func (s *apiclientWhiteboxSuite) newTestCert(c *gc.C, cn string, isCA bool, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	c.Assert(err, jc.ErrorIsNil)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  isCA,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}
	if parent == nil {
		parent, parentKey = tmpl, key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
	c.Assert(err, jc.ErrorIsNil)
	cert, err := x509.ParseCertificate(der)
	c.Assert(err, jc.ErrorIsNil)
	return cert, key
}

// listenTLSChain starts a TLS listener that presents the given DER chain and
// drains connections so probes can read the certificates. It returns the
// listener's address.
func (s *apiclientWhiteboxSuite) listenTLSChain(c *gc.C, key *ecdsa.PrivateKey, leaf *x509.Certificate, chain ...[]byte) string {
	serverCert := tls.Certificate{
		Certificate: chain,
		PrivateKey:  key,
		Leaf:        leaf,
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{serverCert},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { _ = listener.Close() })
	go func() {
		buf := make([]byte, 4)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Read(buf)
			_ = conn.Close()
		}
	}()
	return listener.Addr().String()
}

// TestVerifyCAMultiTrustedViaIntermediates tests the
// server presents a leaf + intermediate whose chain reaches a trusted root
// ONLY through that intermediate.
// verifyCAMulti must accept the chain silently and MUST NOT invoke the VerifyCA
// trust prompt.
func (s *apiclientWhiteboxSuite) TestVerifyCAMultiTrustedViaIntermediates(c *gc.C) {
	root, rootKey := s.newTestCert(c, "root", true, nil, nil)
	intermediate, intKey := s.newTestCert(c, "intermediate", true, root, rootKey)
	leaf, leafKey := s.newTestCert(c, "leaf", false, intermediate, intKey)

	// Serve leaf + intermediate but NOT the root, exactly as a public server
	// relying on a cross-signed root does.
	addr := s.listenTLSChain(c, leafKey, leaf, leaf.Raw, intermediate.Raw)
	serverURL, err := url.Parse("wss://" + addr)
	c.Assert(err, jc.ErrorIsNil)

	// Trust only the root, standing in for the system trust store.
	roots := x509.NewCertPool()
	roots.AddCert(root)

	var verifyCACalled bool
	opts := &dialOpts{
		DialOpts: DialOpts{
			Clock:               clock.WallClock,
			DialAddressInterval: time.Millisecond,
			DNSCache:            nopDNSCache{},
			IPAddrResolver:      net.DefaultResolver,
			VerifyCA: func(host, endpoint string, caCert *x509.Certificate) error {
				verifyCACalled = true
				return errors.New("trust prompt should not be shown for a chain trusted via intermediates")
			},
		},
		certPool: roots,
	}

	err = verifyCAMulti(context.Background(), []*url.URL{serverURL}, opts)
	// The leaf verifies via the served intermediate, so verifyCAMulti returns
	// without ever consulting the prompt.
	c.Check(err, jc.ErrorIsNil)
	c.Check(verifyCACalled, jc.IsFalse)
}

// TestVerifyCAMultiUntrustedInvokesPrompt exercises the whole verifyCAMulti
// flow for an untrusted chain (a self-signed private controller CA): the chain
// cannot be verified against the system roots, so verifyCAMulti delegates to
// the VerifyCA prompt, passing the leaf-most CA.
func (s *apiclientWhiteboxSuite) TestVerifyCAMultiUntrustedInvokesPrompt(c *gc.C) {
	root, rootKey := s.newTestCert(c, "root", true, nil, nil)
	intermediate, intKey := s.newTestCert(c, "intermediate", true, root, rootKey)
	leaf, leafKey := s.newTestCert(c, "leaf", false, intermediate, intKey)

	addr := s.listenTLSChain(c, leafKey, leaf, leaf.Raw, intermediate.Raw)
	serverURL, err := url.Parse("wss://" + addr)
	c.Assert(err, jc.ErrorIsNil)

	var promptedCA *x509.Certificate
	opts := &dialOpts{
		DialOpts: DialOpts{
			Clock:               clock.WallClock,
			DialAddressInterval: time.Millisecond,
			DNSCache:            nopDNSCache{},
			IPAddrResolver:      net.DefaultResolver,
			VerifyCA: func(host, endpoint string, caCert *x509.Certificate) error {
				promptedCA = caCert
				return errors.New("not trusted")
			},
		},
	}

	err = verifyCAMulti(context.Background(), []*url.URL{serverURL}, opts)
	c.Check(err, gc.ErrorMatches, "not trusted")
	// The prompt is shown the leaf-most CA (the intermediate).
	c.Assert(promptedCA, gc.NotNil)
	c.Check(promptedCA.Subject.CommonName, gc.Equals, "intermediate")
}
