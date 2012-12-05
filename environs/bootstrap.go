package environs

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/environs/config"
	"os"
	"path/filepath"
	"time"
)

// Bootstrap bootstraps the given environment.  If the environment does
// not contain a CA certificate, a new certificate and key pair are
// generated, added to the environment configuration, and writeCertAndKey
// will be called to save them.  If writeCertFile is nil, the generated
// certificate and key will be saved to ~/.juju/<environ-name>-cert.pem
// and ~/.juju/<environ-name>-private-key.pem.
//
// If uploadTools is true, the current version of the juju tools will be
// uploaded, as documented in Environ.Bootstrap.
func Bootstrap(environ Environ, uploadTools bool, writeCertAndKey func(environName string, cert, key []byte) error) error {
	if writeCertAndKey == nil {
		writeCertAndKey = writeCertAndKeyToHome
	}
	cfg := environ.Config()
	caCert, hasCACert := cfg.CACert()
	caKey, hasCAKey := cfg.CAPrivateKey()
	if !hasCACert {
		if hasCAKey {
			return fmt.Errorf("environment configuration with CA private key but no certificate")
		}
		var err error
		caCert, caKey, err = cert.NewCA(environ.Name(), time.Now().UTC().AddDate(10, 0, 0))
		if err != nil {
			return err
		}
		m := cfg.AllAttrs()
		m["ca-cert"] = string(caCert)
		m["ca-private-key"] = string(caKey)
		cfg, err = config.New(m)
		if err != nil {
			return fmt.Errorf("cannot create environment configuration with new CA: %v", err)
		}
		if err := environ.SetConfig(cfg); err != nil {
			return fmt.Errorf("cannot set environment configuration with CA: %v", err)
		}
		if err := writeCertAndKey(environ.Name(), caCert, caKey); err != nil {
			return fmt.Errorf("cannot write CA certificate and key: %v", err)
		}
	}
	// Generate a new key pair and certificate for
	// the newly bootstrapped instance.
	cert, key, err := cert.NewServer(environ.Name(), caCert, caKey, time.Now().UTC().AddDate(10, 0, 0))
	if err != nil {
		return fmt.Errorf("cannot generate bootstrap certificate: %v", err)
	}
	return environ.Bootstrap(uploadTools, cert, key)
}

func writeCertAndKeyToHome(name string, cert, key []byte) error {
	path := filepath.Join(os.Getenv("HOME"), ".juju", name)
	if err := ioutil.WriteFile(path+"-cert.pem", cert, 0644); err != nil {
		return err
	}
	if err := ioutil.WriteFile(path+"-private-key.pem", key, 0600); err != nil {
		return err
	}
	return nil
}
