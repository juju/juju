package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
)

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	EnvName     string
	UploadTools bool
}

func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{"bootstrap", "", "start up an environment from scratch", ""}
}

func (c *BootstrapCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools before bootstrapping")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists.
func (c *BootstrapCommand) Run(_ *cmd.Context) error {
	environ, err := environs.NewFromName(c.EnvName)
	if err != nil {
		return err
	}
	// TODO(rog) use (juju/environs).Bootstrap
	return environ.Bootstrap(c.UploadTools, caCertPEM, caKeyPEM)
}

var caCertPEM = []byte(`
-----BEGIN CERTIFICATE-----
MIIBnTCCAUmgAwIBAgIBADALBgkqhkiG9w0BAQUwJjENMAsGA1UEChMEanVqdTEV
MBMGA1UEAxMManVqdSB0ZXN0aW5nMB4XDTEyMTExNDE0Mzg1NFoXDTIyMTExNDE0
NDM1NFowJjENMAsGA1UEChMEanVqdTEVMBMGA1UEAxMManVqdSB0ZXN0aW5nMFow
CwYJKoZIhvcNAQEBA0sAMEgCQQCCOOpn9aWKcKr2GQGtygwD7PdfNe1I9BYiPAqa
I33F5+6PqFdfujUKvoyTJI6XG4Qo/CECaaN9smhyq9DxzMhAgMBAAGjZjBkMA4G
A1UdDwEB/wQEAwIABDASBgNVHRMBAf8ECDAGAQH/AgEBMB0GA1UdDgQWBBQQDswP
FQGeGMeTzPbHW62EZbbTJzAfBgNVHSMEGDAWgBQQDswPFQGeGMeTzPbHW62EZbbT
JzALBgkqhkiG9w0BAQUDQQAqZzN0DqUyEfR8zIanozyD2pp10m9le+ODaKZDDNfH
cB2x26F1iZ8ccq5IC2LtQf1IKJnpTcYlLuDvW6yB96g
-----END CERTIFICATE-----
`)

var caKeyPEM = []byte(`
-----BEGIN RSA PRIVATE KEY-----
MIIBOwIBAAJBAII46mf1pYpwqvYZAa3KDAPs91817Uj0FiI8CprYjfcXn7o+oV1+
NQq+jJMkjpcbhCj8IQJpo32yaHKr0PHMyECAwEAAQJAYctedh4raLE+Ir0a3qnK
pjQSfiUggtYTvTf7+tfAnZu946PX88ysr7XHPkXEGP4tWDTbl8BfGndrTKswVOx6
RQIhAOT5OzafJneDQ5cuGLN/hxIPBLWxKT1/25O6dhtBlRyPAiEAkZfFvCtBZyKB
JFwDdp+7gE98mXtaFrjctLWeFx797U8CIAnnqiMTwWM8H2ljyhfBtYMXeTmu3zzU
hfS4hcNwDiLAiEAkNXXU7YEPkFJD46ps1x7/s0UOutHV8tXZD44ou+l1GkCIQDO
HOzuvYngJpoClGw0ipzJPoNZ2Z/GkdOWGByPeKu/8g==
-----END RSA PRIVATE KEY-----
`)
