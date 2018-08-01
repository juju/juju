// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package https

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// A Logger represents an active logging object to log Client communication
type Logger interface {
	Logf(format string, args ...interface{})
}

// Client represents HTTPS client connection with optional basic authentication
type Client struct {
	protocol         *http.Client
	username         string
	password         string
	connectTimeout   time.Duration
	readWriteTimeout time.Duration
	transport        *http.Transport
	logger           Logger
}

// NewClient returns new Client object with transport configured for https.
// Parameter tlsConfig is optional and can be nil, the default TLSClientConfig of
// http.Transport will be used in this case.
func NewClient(tlsConfig *tls.Config) *Client {
	if tlsConfig == nil {
		tlsConfig = &tls.Config{InsecureSkipVerify: true}
	}

	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	redirectChecker := func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		lastReq := via[len(via)-1]
		if auth := lastReq.Header.Get("Authorization"); len(auth) > 0 {
			req.Header.Add("Authorization", auth)
		}
		return nil
	}

	https := &Client{
		protocol: &http.Client{
			Transport:     tr,
			CheckRedirect: redirectChecker,
		},
		transport: tr,
	}

	tr.Dial = https.dialer

	return https
}

// NewAuthClient returns new Client object with configured https transport
// and attached authentication. Parameter tlsConfig is optional and can be nil, the
// default TLSClientConfig of http.Transport will be used in this case.
func NewAuthClient(username, password string, tlsConfig *tls.Config) *Client {
	https := NewClient(tlsConfig)
	https.username = username
	https.password = password
	return https
}

// ConnectTimeout sets connection timeout
func (c *Client) ConnectTimeout(timeout time.Duration) {
	c.connectTimeout = timeout
	c.transport.CloseIdleConnections()
}

// GetConnectTimeout returns connection timeout for the object
func (c Client) GetConnectTimeout() time.Duration {
	return c.connectTimeout
}

// ReadWriteTimeout sets read-write timeout
func (c *Client) ReadWriteTimeout(timeout time.Duration) {
	c.readWriteTimeout = timeout
}

// GetReadWriteTimeout returns connection timeout for the object
func (c Client) GetReadWriteTimeout() time.Duration {
	return c.readWriteTimeout
}

// Logger sets logger for http traces
func (c *Client) Logger(logger Logger) {
	c.logger = logger
}

// Get performs get request to the url.
func (c Client) Get(url string, query url.Values) (*Response, error) {
	if len(query) != 0 {
		url += "?" + query.Encode()
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if len(c.username) != 0 {
		req.SetBasicAuth(c.username, c.password)
	}

	return c.do(req)
}

// Post performs post request to the url.
func (c Client) Post(url string, query url.Values, body io.Reader) (*Response, error) {
	return c.perform("POST", url, query, body)
}

// Delete performs delete request to the url.
func (c Client) Delete(url string, query url.Values, body io.Reader) (*Response, error) {
	return c.perform("DELETE", url, query, body)
}

func (c Client) perform(request, url string, query url.Values, body io.Reader) (*Response, error) {
	if len(query) != 0 {
		url += "?" + query.Encode()
	}

	if body == nil {
		body = strings.NewReader("{}")
	}

	req, err := http.NewRequest(request, url, body)
	if err != nil {
		return nil, err
	}

	if body != nil {
		h := req.Header
		h.Add("Content-Type", "application/json; charset=utf-8")
	}

	if len(c.username) != 0 {
		req.SetBasicAuth(c.username, c.password)
	}

	return c.do(req)
}

func (c Client) do(r *http.Request) (*Response, error) {
	logger := c.logger

	if logger != nil {
		if buf, err := httputil.DumpRequest(r, true); err == nil {
			logger.Logf("%s", string(buf))
			logger.Logf("")
		}
	}

	readWriteTimeout := c.readWriteTimeout
	if readWriteTimeout > 0 {
		timer := time.AfterFunc(readWriteTimeout, func() {
			c.transport.CancelRequest(r)
		})
		defer timer.Stop()
	}

	var resp *http.Response
	for i := 0; i < 3; i++ {
		if r, err := c.protocol.Do(r); err == nil {
			resp = r
			break
		}
		if logger != nil {
			logger.Logf("broken persistent connection, try [%d], closing idle conns and retry...", i)
		}
		c.transport.CloseIdleConnections()
	}

	if resp == nil {
		return nil, fmt.Errorf("broken connection")
	}

	if logger != nil {
		logger.Logf("HTTP/%s", resp.Status)
		for header, values := range resp.Header {
			logger.Logf("%s: %s", header, strings.Join(values, ","))
		}

		bb, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logger.Logf("failed to read body %s", err)
			return nil, err
		}

		logger.Logf("")
		logger.Logf("%s", string(bb))
		logger.Logf("")

		resp.Body = ioutil.NopCloser(bytes.NewReader(bb))
	}

	return &Response{resp}, nil
}

func (c *Client) dialer(netw, addr string) (net.Conn, error) {
	return net.DialTimeout(netw, addr, c.connectTimeout)
}
