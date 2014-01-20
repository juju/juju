// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	envtesting "launchpad.net/juju-core/environs/testing"
	jp "launchpad.net/juju-core/provider/joyent"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"

	"launchpad.net/gojoyent/jpc"
	localmanta "launchpad.net/gojoyent/localservices/manta"
)

const (
	testUser        = "test"
	testKeyFileName = "provider_id_rsa"
	testPrivateKey  = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAza+KvczCrcpQGRq9e347VHx9oEvuhseJt0ydR+UMAveyQprU
4JHvzwUUhGnG147GJQYyfQ4nzaSG62az/YThoZJzw8gtxGkVHv0wlAlRkYhxbKbq
8WQIh73xDQkHLw2lXLvf7Tt0Mhow0qGEmkOjTb5fPsj2evphrV3jJ15QlhL4cv33
t8jVadIrL0iIpwdqWiPqUKpsrSfKJghkoXS6quPy78P820TnuoBG+/Ppr8Kkvn6m
A7j4xnOQ12QE6jPK4zkikj5ZczSC4fTG0d3BwwX4VYu+4y/T/BX0L9VNUmQU22Y+
/MRXAUZxsa8VhNB+xXF5XSubyb2n6loMWWaYGwIDAQABAoIBAQDCJt9JxYxGS+BL
sigF9+O9Hj3fH42p/5QJR/J2uMgbzP+hS1GCIX9B5MO3MbmWI5j5vd3OmZwMyy7n
6Wwg9FufDgTkW4KIEcD0HX7LXfh27VpTe0PuU8SRjUOKUGlNiw36eQUog6Rs3rgT
Oo9Wpl3xtq9lLoErGEk3QpZ2xNpArTfsN9N3pdmD4sv7wmJq0PZQyej482g9R0g/
5k2ni6JpcEifzBQ6Bzx3EV2l9UipEIqbqDpMOtYFCpnLQhEaDfUribqXINGIsjiq
VyFa3Mbg/eayqG3UX3rVTCif2NnW2ojl4mMgWCyxgWfb4Jg1trc3v7X4SXfbgPWD
WcfrOhOhAoGBAP7ZC8KHAnjujwvXf3PxVNm6CTs5evbByQVyxNciGxRuOomJIR4D
euqepQ4PuNAabnrbMyQWXpibIKpmLnBVoj1q0IMXYvi2MZF5e2tH/Gx01UvxamHh
bKhHmp9ImHhVl6kObXOdNvLVTt/BI5FZBblvm7qOoiVwImPbqqVHP7Q5AoGBAM6d
mNsrW0iV/nP1m7d8mcFw74PI0FNlNdfUoePUgokO0t5OU0Ri/lPBDCRGlvVF3pj1
HnmwroNtdWr9oPVB6km8193fb2zIWe53tj+6yRFQpz5elrSPfeZaZXlJZAGCCCdN
gBggWQFPeQiT54aPywPpcTZHIs72XBqQ6QsIPrbzAoGAdW2hg5MeSobyFuzHZ69N
/70/P7DuvgDxFbeah97JR5K7GmC7h87mtnE/cMlByXJEcgvK9tfv4rWoSZwnzc9H
oLE1PxJpolyhXnzxp69V2svC9OlasZtjq+7Cip6y0s/twBJL0Lgid6ZeX6/pKbIx
dw68XSwX/tQ6pHS1ns7DxdECgYBJbBWapNCefbbbjEcWsC+PX0uuABmP2SKGHSie
ZrEwdVUX7KuIXMlWB/8BkRgp9vdAUbLPuap6R9Z2+8RMA213YKUxUiotdREIPgBE
q2KyRX/5GPHjHi62Qh9XN25TXtr45ICFklEutwgitTSMS+Lv8+/oQuUquL9ILYCz
C+4FYwKBgQDE9yZTUpJjG2424z6bl/MHzwl5RB4pMronp0BbeVqPwhCBfj0W5I42
1ZL4+8eniHfUs4GXzf5tb9YwVt3EltIF2JybaBvFsv2o356yJUQmqQ+jyYRoEpT5
2SwilFo/XCotCXxi5n8sm9V94a0oix4ehZrohTA/FZLsggwFCPmXfw==
-----END RSA PRIVATE KEY-----`
	testKeyFingerprint = "66:ca:1c:09:75:99:35:69:be:91:08:25:03:c0:17:c0"
)

func TestJoyentProvider(t *stdtesting.T) {
	gc.TestingT(t)
}

type localMantaService struct {
	creds      *jpc.Credentials
	Server     *httptest.Server
	Mux        *http.ServeMux
	oldHandler http.Handler
	manta      *localmanta.Manta
}

func (s *localMantaService) Start(c *gc.C) {
	// Set up the HTTP server.
	s.Server = httptest.NewServer(nil)
	c.Assert(s.Server, gc.NotNil)
	s.oldHandler = s.Server.Config.Handler
	s.Mux = http.NewServeMux()
	s.Server.Config.Handler = s.Mux

	// Set up a Joyent Manta service.
	auth := jpc.Auth{User: testUser, KeyFile: testKeyFileName, Algorithm: "rsa-sha256"}

	s.creds = &jpc.Credentials{
		UserAuthentication: auth,
		MantaKeyId:         testKeyFingerprint,
		MantaEndpoint:      jpc.Endpoint{URL: s.Server.URL},
	}
	s.manta = localmanta.New(s.creds.MantaEndpoint.URL, s.creds.UserAuthentication.User)
	s.manta.SetupHTTP(s.Mux)
	c.Logf("Started local Manta service at: %v", s.Server.URL)
}

func (s *localMantaService) Stop() {
	s.Mux = nil
	s.Server.Config.Handler = s.oldHandler
	s.Server.Close()
}

type providerSuite struct {
	testbase.LoggingSuite
	envtesting.ToolsFixture
	restoreTimeouts func()
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpSuite(c *gc.C) {
	s.restoreTimeouts = envtesting.PatchAttemptStrategies()
	s.LoggingSuite.SetUpSuite(c)
	createTestKey()
}

func (s *providerSuite) TearDownSuite(c *gc.C) {
	removeTestKey()
	s.restoreTimeouts()
	s.LoggingSuite.TearDownSuite(c)
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
}

func (s *providerSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func GetFakeConfig(sdcUrl, mantaUrl string) coretesting.Attrs {
	return coretesting.FakeConfig().Merge(coretesting.Attrs{
		"name":         "joyent test environment",
		"type":         "joyent",
		"sdc-user":     testUser,
		"sdc-key-id":   testKeyFingerprint,
		"sdc-url":      sdcUrl,
		"manta-user":   testUser,
		"manta-key-id": testKeyFingerprint,
		"manta-url":    mantaUrl,
		"key-file":     fmt.Sprintf("%s/.ssh/%s", os.Getenv("HOME"), testKeyFileName),
		"algorithm":    "rsa-sha256",
		"control-dir":  "juju-test",
	})
}

// makeEnviron creates a functional Joyent environ for a test.
func (suite *providerSuite) makeEnviron(sdcUrl, mantaUrl string) *jp.JoyentEnviron {
	/*attrs := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"name":         "joyent test environment",
		"type":         "joyent",
		"sdc-user":     "dstroppa",
		"sdc-key-id":   "12:c3:a7:cb:a2:29:e2:90:88:3f:04:53:3b:4e:75:40",
		"sdc-url":      "https://us-west-1.api.joyentcloud.com",
		"manta-user":   "dstroppa",
		"manta-key-id": "12:c3:a7:cb:a2:29:e2:90:88:3f:04:53:3b:4e:75:40",
		"manta-url":    "https://us-east.manta.joyent.com",
		"key-file":     fmt.Sprintf("%s/.ssh/id_rsa", os.Getenv("HOME")),
		"algorithm":    "rsa-sha256",
		"control-dir":  "juju-test",
	})*/

	attrs := GetFakeConfig(sdcUrl, mantaUrl)
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		panic(err)
	}
	env, err := jp.NewEnviron(cfg)
	if err != nil {
		panic(err)
	}
	return env
}

func createTestKey() error {
	keyFile := fmt.Sprintf("%s/.ssh/%s", os.Getenv("HOME"), testKeyFileName)
	return ioutil.WriteFile(keyFile, []byte(testPrivateKey), 400)
}

func removeTestKey() error {
	keyFile := fmt.Sprintf("%s/.ssh/%s", os.Getenv("HOME"), testKeyFileName)
	return os.Remove(keyFile)
}
