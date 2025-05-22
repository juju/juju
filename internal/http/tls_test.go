// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/juju/tc"
)

type TLSSuite struct{}

func TestTLSSuite(t *testing.T) {
	tc.Run(t, &TLSSuite{})
}

func (TLSSuite) TestWinCipher(c *tc.C) {
	if runtime.GOOS != "windows" {
		c.Skip("Windows-specific test.")
	}

	d := c.MkDir()
	go runServer(d, c)

	out := filepath.Join(d, "out.txt")

	// this script enables TLS 1.2, accepts whatever cert the server has (since
	// it's self-signed), then tries to connect to the web server.
	script := fmt.Sprintf(`[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
[System.Net.ServicePointManager]::ServerCertificateValidationCallback = {$true}
(New-Object System.Net.WebClient).DownloadFile("https://127.0.0.1:10443", "%s")
`, out)
	err := runPS(d, script)
	c.Assert(err, tc.ErrorIsNil)
	b, err := ioutil.ReadFile(out)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(b), tc.Equals, "This is an example server.\n")
}

func runServer(dir string, c *tc.C) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("This is an example server.\n"))
	})

	s := http.Server{
		Addr:      ":10443",
		TLSConfig: SecureTLSConfig(),
		Handler:   handler,
	}

	certFile := filepath.Join(dir, "cert.pem")
	err := ioutil.WriteFile(certFile, []byte(cert), 0600)
	c.Assert(err, tc.ErrorIsNil)
	keyFile := filepath.Join(dir, "key.pem")
	err = ioutil.WriteFile(keyFile, []byte(key), 0600)
	c.Assert(err, tc.ErrorIsNil)

	err = s.ListenAndServeTLS(certFile, keyFile)
	c.Assert(err, tc.ErrorIsNil)
}

func runPS(dir, script string) error {
	scriptFile := filepath.Join(dir, "script.ps1")
	args := []string{
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "RemoteSigned",
		"-File", scriptFile,
	}
	// Exceptions don't result in a non-zero exit code by default
	// when using -File. The exit code of an explicit "exit" when
	// using -Command is ignored and results in an exit code of 1.
	// We use -File and trap exceptions to cover both.
	script = "trap {Write-Error $_; exit 1}\n" + script
	if err := ioutil.WriteFile(scriptFile, []byte(script), 0600); err != nil {
		return err
	}
	cmd := exec.Command("powershell.exe", args...)
	return cmd.Run()
}

const (
	cert = `-----BEGIN CERTIFICATE-----
MIIC9TCCAd2gAwIBAgIRALhL8rNhi3x29T8g/AwK9bAwDQYJKoZIhvcNAQELBQAw
EjEQMA4GA1UEChMHQWNtZSBDbzAeFw0xNjA3MjYxNjI4MzRaFw0xNzA3MjYxNjI4
MzRaMBIxEDAOBgNVBAoTB0FjbWUgQ28wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAw
ggEKAoIBAQCaVIZUmQdBTXYATbTmMhscCTUSNt+Dn3OP8w2v/2QJyUz3s1eiiuec
ymD+6TC7lNjzIXhFJnHTyuo/p2d2lHNvbQmUh0kMjPxnIDaCqWZXcjR+vnFo4jgl
VtxCqPG2zi62kZxB0Pu9DzJ7AlqF9BTbpu0INDyFzLJtj73RIv00kRDTpFzHQSNN
tzi9ZzKY7ZS6urftqXc4pqoaSyFXqw7uSNcBcr7Cc8oXIz5tQoVU5m0uKBGOQvwC
b+ICd+RIYS09L1E76UGpDcrJ0LQlysQ/ZMmSsA5YHGf5KE+N0WnWdQCADq3voQra
q47HBpH+ByA1F1REMwgMoFNZRNrEHdXFAgMBAAGjRjBEMA4GA1UdDwEB/wQEAwIF
oDATBgNVHSUEDDAKBggrBgEFBQcDATAMBgNVHRMBAf8EAjAAMA8GA1UdEQQIMAaH
BH8AAAEwDQYJKoZIhvcNAQELBQADggEBAJPwxR3fhEpZz2JB2dAUuj0KqFD7uPQp
m30Slu3cihqQkoaGiSMQdGSZ/VnieHbS/XaZo8JqixU8RucYjVT2eM5YRgcGxU91
L4yJfPm7qPwGIvwpfqlZK5GcpC/qk3joNqL43gGfn6vbtqw+wF33yfcyTlTO1hwN
vZSU4HC3Hz+FoFnmqkW5lXiuggm/jsdWqPIDA0NJHrws/wjqu3T+wQcfTvIwIPMG
WFmUP5hvWD/9HpizJqROhRZwfsJHDpHDu0nKgSDnV1gX2S5XaUsUWu53V/Hczbo0
fSD4wg+Zd/x3fh+EpOd1qbHmXrDWSs4z/T61yKzrgENd/kSncJC38pg=
-----END CERTIFICATE-----
`
	key = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAmlSGVJkHQU12AE205jIbHAk1Ejbfg59zj/MNr/9kCclM97NX
oornnMpg/ukwu5TY8yF4RSZx08rqP6dndpRzb20JlIdJDIz8ZyA2gqlmV3I0fr5x
aOI4JVbcQqjxts4utpGcQdD7vQ8yewJahfQU26btCDQ8hcyybY+90SL9NJEQ06Rc
x0EjTbc4vWcymO2Uurq37al3OKaqGkshV6sO7kjXAXK+wnPKFyM+bUKFVOZtLigR
jkL8Am/iAnfkSGEtPS9RO+lBqQ3KydC0JcrEP2TJkrAOWBxn+ShPjdFp1nUAgA6t
76EK2quOxwaR/gcgNRdURDMIDKBTWUTaxB3VxQIDAQABAoIBAQCBktX10UW2HkMk
nhlz7D22nERito+TAx0Tjw2+5r4nOUvV7E13uwgbLA+j9kVkOOStvTwtUsne+E8U
gojrllgVBYc1nSBH2VdRfkpGCdRTNx+8CklNtiFNuE/V5+KJiTLPNhHrcHrrkQbh
IGjAbt3UTaJVcQYfkG1+b2D/ZlERAC0s0lnqidoqeAaiXDmDesIz+gXkpfbt5mHa
f/LRFRvjtBDjCOTkZ3OdFeSyW+z4zs75vvk3amQNixGW74obFUZFBvF81yUZH7kf
bWBMJMIo024oo4Rpi5k279gx2pWNLHQ68AWF/zLbu32xGrSQuTelVU5MNgEDVB9W
3T01iHwBAoGBAMGGslxNYcf2lg0pW6II4EmOSvbdZ5z9kmV92wkN4zTP3Tzr/Kzf
UMALczvCBYplo6Q6nR+TvRukl8Mr1e5m7Ophfv21vZfprs2YXigL9vTZKRsis8Fk
QSK2kO9CVnWjFu11jYCDN9nUD+9lB+ry9grdY0744a8dTsxmZ1m1ZwA1AoGBAMwm
nF0+OnMkILfsnaK6PVsJBUI5N/j05P/mDQcDZdQMVOBSh/kceQ4LWHXdL0lMVLBY
pGPXqwsO8Q/d2R2oI1acgIFcl73FTchrQd1YaHmnyfqInhKt9QOXj1c0ii4BL3ff
iGVf4gqQVH0B2nK7pjkBlwvpjsYFVDHP9/xkXlFRAoGAC9mgoFBItYLe601mBAUB
Ht/srTMffhh012wedm54RCqaRHm6zicafbf1xWn7Bt90ZsEEEAPu53tro5LSlbeN
uEhiC00On/e6MXKsCU26QIHvp263jRcDegmt1Ei+nJNw+vdgw8bFK7x1gVYxZuyb
rkyiIRrSTvO/eHqox3B5LyUCgYAmKZWTTJ2qhndjSmURVVVA3kfQYFfZPxZLy9pl
lDoF0KRRJrxqUetDN9W6erVrM0ylhnx8eYVs1Mc1WxhKFfM9LpZLGF75R5fJvlsa
oHsvOrFkFwPNpB0oJb3S5GxsOyZ/dxbNNIZRyTcyAxWt2uwwvd5ZiLh6xeY+RY0q
7iw/cQKBgQCaWJ8bSNNhQeaBSW5IVHFctYtLPv9aPHagBdJkKmeb06HWQHi+AvkY
nd0dgM/TfgtnuhbVS4ISkT4vZoSn84hOE7BG5rSPE+/q24Wv5gG0PI1sky8tmXzX
juAEWSJVCSE0TK/mvBVdlyKOJoEgtfMcRfDQfA1rI9My0rU+/Y5A0w==
-----END RSA PRIVATE KEY-----`
)

func (TLSSuite) TestDisableKeepAlives(c *tc.C) {
	transport := DefaultHTTPTransport()
	c.Assert(transport.DisableKeepAlives, tc.Equals, false)

	transport = NewHTTPTLSTransport(TransportConfig{})
	c.Assert(transport.DisableKeepAlives, tc.Equals, false)

	transport = NewHTTPTLSTransport(TransportConfig{
		DisableKeepAlives: true,
	})
	c.Assert(transport.DisableKeepAlives, tc.Equals, true)
}
