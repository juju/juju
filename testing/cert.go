package testing

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

func verifyCertificates() error {
	_, err := tls.X509KeyPair([]byte(CACertPEM), []byte(CAKeyPEM))
	if err != nil {
		return fmt.Errorf("bad CA cert key pair: %v", err)
	}
	_, err = tls.X509KeyPair([]byte(serverCertPEM), []byte(serverKeyPEM))
	if err != nil {
		return fmt.Errorf("bad server cert key pair: %v", err)
	}
	caCert, err := trivial.ParseCertificate([]byte(CACertPEM))
	if err != nil {
		return err
	}
	serverCert, err := trivial.ParseCertificate([]byte(serverCertPEM))
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	opts := x509.VerifyOptions{
		DNSName: "anything",
		Roots: pool,
	}
	_, err = serverCert.Verify(opts)
	if err != nil {
		return fmt.Errorf("verification failed: %v", err)
	}
	return nil
}

func init() {
//	if err := verifyCertificates(); err != nil {
//		panic(err)
//	}
}

// CACertPEM and CAKeyPEM make up a CA key pair.
// CACertX509 and CAKeyRSA hold their parsed equivalents.
var (
	CACertPEM = `
-----BEGIN CERTIFICATE-----
MIIBnTCCAUmgAwIBAgIBADALBgkqhkiG9w0BAQUwJjENMAsGA1UEChMEanVqdTEV
MBMGA1UEAxMManVqdSB0ZXN0aW5nMB4XDTEyMTExNDE0Mzg1NFoXDTIyMTExNDE0
NDM1NFowJjENMAsGA1UEChMEanVqdTEVMBMGA1UEAxMManVqdSB0ZXN0aW5nMFow
CwYJKoZIhvcNAQEBA0sAMEgCQQCCOOpn9aWKcKr2GQGtygwD7PdfNe1I9BYiPAqa
2I33F5+6PqFdfujUKvoyTJI6XG4Qo/CECaaN9smhyq9DxzMhAgMBAAGjZjBkMA4G
A1UdDwEB/wQEAwIABDASBgNVHRMBAf8ECDAGAQH/AgEBMB0GA1UdDgQWBBQQDswP
FQGeGMeTzPbHW62EZbbTJzAfBgNVHSMEGDAWgBQQDswPFQGeGMeTzPbHW62EZbbT
JzALBgkqhkiG9w0BAQUDQQAqZzN0DqUyEfR8zIanozyD2pp10m9le+ODaKZDDNfH
8cB2x26F1iZ8ccq5IC2LtQf1IKJnpTcYlLuDvW6yB96g
-----END CERTIFICATE-----
`[1:]

	CACertX509 = mustParseCertPEM(CACertPEM)

	CAKeyPEM = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOwIBAAJBAII46mf1pYpwqvYZAa3KDAPs91817Uj0FiI8CprYjfcXn7o+oV1+
6NQq+jJMkjpcbhCj8IQJpo32yaHKr0PHMyECAwEAAQJAYctedh4raLE+Ir0a3qnK
pjQSfiUggtYTvTf7+tfAnZu946PX88ysr7XHPkXEGP4tWDTbl8BfGndrTKswVOx6
RQIhAOT5OzafJneDQ5cuGLN/hxIPBLWxKT1/25O6dhtBlRyPAiEAkZfFvCtBZyKB
JFwDdp+7gE98mXtaFrjctLWeFx797U8CIAnnqiMTwWM8H2ljyhfBtYMXeTmu3zzU
0hfS4hcNwDiLAiEAkNXXU7YEPkFJD46ps1x7/s0UOutHV8tXZD44ou+l1GkCIQDO
HOzuvYngJpoClGw0ipzJPoNZ2Z/GkdOWGByPeKu/8g==
-----END RSA PRIVATE KEY-----
`[1:]
	CAKeyRSA = mustParseKeyPEM(CAKeyPEM)

	// serverCertPEM holds a certificate siged by the above CA cert.
	serverCertPEM = `
-----BEGIN CERTIFICATE-----
MIIBfDCCASigAwIBAgIBADALBgkqhkiG9w0BAQUwJjENMAsGA1UEChMEanVqdTEV
MBMGA1UEAxMManVqdSB0ZXN0aW5nMB4XDTEyMTEyNDEzMDY1OVoXDTIyMTEyNDEz
MTE1OVowGjEMMAoGA1UEChMDaG1tMQowCAYDVQQDEwEqMFkwCwYJKoZIhvcNAQEB
A0oAMEcCQF9KBtClwqaJuvhRKNNdsxyrdVTfgNhLTf1DX+Z3iBTpvb8fxihC9xQv
voslONe+wL1MQi8QkjUzex1Z7abC+m8CAwEAAaNSMFAwDgYDVR0PAQH/BAQDAgAQ
MB0GA1UdDgQWBBRUQs95lLcaqz6iGce/APLVfdw5ZjAfBgNVHSMEGDAWgBRQqPrU
s3Mlim0tNfp20ruYuj6LTTALBgkqhkiG9w0BAQUDQQBBBuMUIKFpSVjhm1ybbHnC
BP6lvBILWjb6h7f0hFFQQq2Ks8Hr1cwoRNQQFe06qIb7GFhwu6RoY3BDRPAQbZG5
-----END CERTIFICATE-----
`[1:]

	// serverKeyPEM holds the private key for serverCertPEM.
	serverKeyPEM = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOQIBAAJAX0oG0KXCpom6+FEo012zHKt1VN+A2EtN/UNf5neIFOm9vx/GKEL3
FC++iyU4177AvUxCLxCSNTN7HVntpsL6bwIDAQABAkBP3qifspDZLpC9GqnxoJRE
76JFJaHFqjkQk6yKbJ5viAUU+rrsKPuU8Sk1oP005QtzofWQKKP8dLZg50dCpDWB
AiEAjViPqgn2tYt/64xJDUjOK1fMpY/yiK0aEmXFXTctgX8CIQCslXcTO03XeZpD
0WJDDmEeex2gwAJC2SqtH3XOL3EfEQIgaNWmuJdrRHuTBUGnbRLy13LndmStnnHF
RJ/3IowqVEECIQCVPzBZdkir1aJdkZ47RR0hwfBuSn3qF2m7i2BSLV7TMQIgR/0Q
TgZwrr9JK+c8N+/YQ8zMv85a4POQHZnNHrVKeRQ=
-----END RSA PRIVATE KEY-----
`[1:]
)


func mustParseCertPEM(pemData string) *x509.Certificate {
	b, _ := pem.Decode([]byte(pemData))
	if b.Type != "CERTIFICATE" {
		panic("unexpected type")
	}
	cert, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		panic(err)
	}
	return cert
}

func mustParseKeyPEM(pemData string) *rsa.PrivateKey {
	b, _ := pem.Decode([]byte(pemData))
	if b.Type != "RSA PRIVATE KEY" {
		panic("unexpected type")
	}
	key, err := x509.ParsePKCS1PrivateKey(b.Bytes)
	if key != nil {
		return key
	}
	key1, err := x509.ParsePKCS8PrivateKey(b.Bytes)
	if err != nil {
		panic(err)
	}
	return key1.(*rsa.PrivateKey)
}
