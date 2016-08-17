// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	gc "gopkg.in/check.v1"

	envtesting "github.com/juju/juju/environs/testing"
	coretesting "github.com/juju/juju/testing"
)

const (
	testUser       = "test"
	testPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
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

type baseSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
	restoreTimeouts func()
}

var _ = gc.Suite(&baseSuite{})

func (s *baseSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	s.restoreTimeouts = envtesting.PatchAttemptStrategies()
}

func (s *baseSuite) TearDownSuite(c *gc.C) {
	s.restoreTimeouts()
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
}

func (s *baseSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func GetFakeConfig() coretesting.Attrs {
	return coretesting.FakeConfig().Merge(coretesting.Attrs{
		"name":          "joyent-test-model",
		"type":          "joyent",
		"agent-version": coretesting.FakeVersionNumber.String(),
	})
}
