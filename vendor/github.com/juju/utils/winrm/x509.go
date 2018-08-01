// Copyright 2016 Canonical ltd.
// Copyright 2016 Cloudbase solutions
// Licensed under the lgplv3, see licence file for details.

package winrm

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/cert"
)

// List of exported internal errors
var (
	errNoClientCert       = errors.New("No client winrm cert inside dir")
	errNoClientPrivateKey = errors.New("No client private key inside dir")
	errNoX509Folder       = errors.New("No configuration x509 folder is present in this location")
)

// X509 type that defines windows remote manager
// credentials for client-server secure communication
type X509 struct {
	mu sync.Mutex

	cert   []byte // client cert
	key    []byte // client private key
	cacert []byte // ca server cert

}

// NewX509 returns a new to an empty X509
func NewX509() *X509 {
	return &X509{}
}

// LoadClientCert generates client cert for x509 authentication
// if the directory files are not already there , if they are already there
// it will load them into memory
func (x *X509) LoadClientCert(certFile, keyFile string) error {
	x.mu.Lock()
	defer x.mu.Unlock()

	b1, key := filepath.Split(keyFile)
	b2, cert := filepath.Split(certFile)
	if strings.Compare(b1, b2) != 0 {
		return fmt.Errorf("Cert and Key base paths dosen't match")
	}

	base, err := utils.NormalizePath(b1)
	if err != nil {
		return err
	}
	logger.Debugf("Init winrm credentials path for the module %s", base)
	logger.Debugf("Init winrm path key %s", keyFile)
	logger.Debugf("Init winrm path cert %s", certFile)

	if err = x.read(base, key, cert); err != nil &&
		err != errNoClientCert &&
		err != errNoX509Folder &&
		err != errNoClientPrivateKey {
		return err
	}

	if err == errNoClientCert ||
		err == errNoX509Folder ||
		err == errNoClientPrivateKey {
		if err = os.RemoveAll(base); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(base, 0700); err != nil {
		return err
	}
	if err = x.write(base, key, cert); err != nil {
		return err
	}

	return nil
}

// LoadCACert reads ca cert into memory
func (x *X509) LoadCACert(path string) error {
	var err error
	x.cacert, err = ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("No CA detected in the path %s, this defaults to use insecure option for https", path)
	}
	return nil
}

// write generates the client cert/key pair and writes them on disk
func (x *X509) write(base, key, cert string) error {

	if x.key != nil && x.cert != nil {
		return nil
	}

	var err error
	logger.Debugf("Generating winrm cert and private key")
	x.cert, x.key, err = newCredentials()
	if err != nil {
		return err
	}

	logger.Debugf("Writing newly generated winrm cert and private key")
	key, cert = filepath.Join(base, key), filepath.Join(base, cert)
	if err = ioutil.WriteFile(key, x.key, 0644); err != nil {
		return err
	}

	if err = ioutil.WriteFile(cert, x.cert, 0644); err != nil {
		return err
	}

	return nil
}

// read reads from disk client cert,key pair
func (x *X509) read(base, keyFile, certFile string) error {
	keyFile = filepath.Join(base, keyFile)
	certFile = filepath.Join(base, certFile)

	var err error
	if err = confExists(base, keyFile, certFile); err != nil {
		return err
	}

	logger.Debugf("Reading winrm private key and cert")
	if x.cert, err = ioutil.ReadFile(certFile); err != nil {
		return err
	}

	if x.key, err = ioutil.ReadFile(keyFile); err != nil {
		return err
	}

	return nil
}

// confExists checks whenever the conf folder and files already exists or not.
func confExists(base, key, cert string) error {
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return errNoX509Folder
	}

	if _, err := os.Stat(cert); os.IsNotExist(err) {
		return errNoClientCert
	}

	if _, err := os.Stat(key); os.IsNotExist(err) {
		return errNoClientPrivateKey
	}

	return nil
}

// newCredentials makes winrm RSA Cert and Key
// with default configuration for winrm juju connections
func newCredentials() ([]byte, []byte, error) {
	now := time.Now()
	expiry := now.AddDate(10, 0, 0) // 10 years is enough
	cert, key, err := cert.NewClientCert(
		fmt.Sprintf("juju-generated client cert for model %s", "Administrator"),
		"", expiry, 2048)
	return []byte(cert), []byte(key), err
}

// Reset fills up the internal state with nil values
func (x *X509) Reset() {
	x.key = nil
	x.cert = nil
	x.cacert = nil
	logger.Debugf("Reseting the internal winrm cert and key state")
}

// ClientCert returns the internal credential client x509 cert
func (x *X509) ClientCert() []byte {
	return x.cert
}

// ClientKey returns the internal credential client x509 private key
func (x *X509) ClientKey() []byte {
	return x.key
}

// CACert returns the internal credential client ca cert
func (x *X509) CACert() []byte {
	return x.cacert
}
