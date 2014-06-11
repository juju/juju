// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"fmt"

	"github.com/juju/juju/worker/localstorage"
)

// storageConfig is an struct implementing LocalTLSStorageConfig interface
// to support serving storage over TLS.
type storageConfig struct {
	ecfg        *environConfig
	storageDir  string
	storageAddr string
	storagePort int
}

var _ localstorage.LocalTLSStorageConfig = (*storageConfig)(nil)

// StorageDir is a storage local directory
func (c storageConfig) StorageDir() string {
	return c.storageDir
}

// StorageAddr is a storage IP address and port
func (c storageConfig) StorageAddr() string {
	return fmt.Sprintf("%s:%d", c.storageAddr, c.storagePort)
}

// StorageCACert is the CA certificate in PEM format.
func (c storageConfig) StorageCACert() string {
	if cert, ok := c.ecfg.CACert(); ok {
		return cert
	}
	return ""
}

// StorageCAKey is the CA private key in PEM format.
func (c storageConfig) StorageCAKey() string {
	if key, ok := c.ecfg.CAPrivateKey(); ok {
		return key
	}
	return ""
}

// StorageHostnames is the set of hostnames that will
// be assigned to the storage server's certificate.
func (c storageConfig) StorageHostnames() []string {
	return []string{c.storageAddr}
}

// StorageAuthKey is the key that clients must present
// to perform modifying operations.
func (c storageConfig) StorageAuthKey() string {
	return c.ecfg.storageAuthKey()
}
