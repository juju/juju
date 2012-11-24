package trivial
import (
	"crypto/x509"
	"encoding/pem"
	"errors"
)

func ParseCertificate(certPEM []byte) (*x509.Certificate, error) {
	for len(certPEM) > 0 {
		var certBlock *pem.Block
		certBlock, certPEM = pem.Decode(certPEM)
		if certBlock == nil {
			break
		}
		if certBlock.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(certBlock.Bytes)
			return cert, err
		}
	}
	return nil, errors.New("no certificates found")
}
