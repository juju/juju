package environs

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"os"
	"path/filepath"
	"time"
)

// Bootstrap bootstraps the given environment.  If the environment does
// not contain a CA certificate, a new certificate and key pair are
// generated, added to the environment configuration, and writeCertFile
// will be called to save them.  If writeCertFile is nil, the generated
// certificate and key pair will be saved to ~/.juju.
//
// If uploadTools is true, the current version of the juju tools will be
// uploaded, as documented in Environ.Bootstrap.
func Bootstrap(environ Environ, uploadTools bool, writeCertFile func(name string, data []byte) error) error {
	if writeCertFile == nil {
		writeCertFile = writeCertFileToHome
	}
	cfg := environ.Config()
	caCertPEM, hasCACert := cfg.CACert()
	caKeyPEM, hasCAKey := cfg.CAPrivateKey()
	log.Printf("got cert and key: %v, %v", hasCACert, hasCAKey)
	if !hasCACert {
		if hasCAKey {
			return fmt.Errorf("environment config has private key without CA certificate")
		}
		log.Printf("generating new CA certificate")
		var err error
		caCertPEM, caKeyPEM, err = cert.NewCA(environ.Name(), time.Now().UTC().AddDate(10, 0, 0))
		if err != nil {
			return err
		}
		m := cfg.AllAttrs()
		m["ca-cert"] = string(caCertPEM)
		m["ca-private-key"] = string(caKeyPEM)
		cfg, err = config.New(m)
		if err != nil {
			return fmt.Errorf("cannot create config with added CA certificate: %v", err)
		}
		if err := environ.SetConfig(cfg); err != nil {
			return fmt.Errorf("cannot add CA certificate to environ: %v", err)
		}
		if err := writeCertFile(environ.Name()+"-cert.pem", caCertPEM); err != nil {
			return fmt.Errorf("cannot save CA certificate: %v", err)
		}
		if err := writeCertFile(environ.Name()+"-private-key.pem", caKeyPEM); err != nil {
			return fmt.Errorf("cannot save CA key: %v", err)
		}
	}
	// Generate a new key pair and certificate for
	// the newly bootstrapped instance.
	certPEM, keyPEM, err := cert.NewServer(environ.Name(), caCertPEM, caKeyPEM, time.Now().UTC().AddDate(10, 0, 0))
	if err != nil {
		return fmt.Errorf("cannot generate bootstrap certificate: %v", err)
	}
	return environ.Bootstrap(uploadTools, certPEM, keyPEM)
}

func writeCertFileToHome(name string, data []byte) error {
	path := filepath.Join(os.Getenv("HOME"), ".juju", name)
	return ioutil.WriteFile(path, data, 0600)
}
