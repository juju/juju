// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package localstorage

import (
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent"
)

const (
	// TODO(axw) 2013-09-25 bug #1230131
	// Move these variables out of agent when we can do upgrades in
	// the right place. In this case, the local provider should do
	// the envvar-to-agent.conf migration.
	StorageDir       = agent.StorageDir
	StorageAddr      = agent.StorageAddr
	StorageCACert    = "StorageCACert"
	StorageCAKey     = "StorageCAKey"
	StorageHostnames = "StorageHostnames"
	StorageAuthKey   = "StorageAuthKey"
)

// LocalStorageConfig is an interface that, if implemented, may be used
// to configure a machine agent for use with the localstorage worker in
// this package.
type LocalStorageConfig interface {
	StorageDir() string
	StorageAddr() string
}

// LocalTLSStorageConfig is an interface that extends LocalStorageConfig
// to support serving storage over TLS.
type LocalTLSStorageConfig interface {
	LocalStorageConfig

	// StorageCACert is the CA certificate in PEM format.
	StorageCACert() []byte

	// StorageCAKey is the CA private key in PEM format.
	StorageCAKey() []byte

	// StorageHostnames is the set of hostnames that will
	// be assigned to the storage server's certificate.
	StorageHostnames() []string

	// StorageAuthKey is the key that clients must present
	// to perform modifying operations.
	StorageAuthKey() string
}

type config struct {
	storageDir  string
	storageAddr string
	caCertPEM   []byte
	caKeyPEM    []byte
	hostnames   []string
	authkey     string
}

// StoreConfig takes a LocalStorageConfig (or derivative interface),
// and stores it in a map[string]string suitable for updating an
// agent.Config's key/value map.
func StoreConfig(storageConfig LocalStorageConfig) (map[string]string, error) {
	kv := make(map[string]string)
	kv[StorageDir] = storageConfig.StorageDir()
	kv[StorageAddr] = storageConfig.StorageAddr()
	if tlsConfig, ok := storageConfig.(LocalTLSStorageConfig); ok {
		if authkey := tlsConfig.StorageAuthKey(); authkey != "" {
			kv[StorageAuthKey] = authkey
		}
		if cert := tlsConfig.StorageCACert(); cert != nil {
			kv[StorageCACert] = string(cert)
		}
		if key := tlsConfig.StorageCAKey(); key != nil {
			kv[StorageCAKey] = string(key)
		}
		if hostnames := tlsConfig.StorageHostnames(); len(hostnames) > 0 {
			data, err := goyaml.Marshal(hostnames)
			if err != nil {
				return nil, err
			}
			kv[StorageHostnames] = string(data)
		}
	}
	return kv, nil
}

func loadConfig(agentConfig agent.Config) (*config, error) {
	config := &config{
		storageDir:  agentConfig.Value(StorageDir),
		storageAddr: agentConfig.Value(StorageAddr),
		authkey:     agentConfig.Value(StorageAuthKey),
	}

	caCertPEM := agentConfig.Value(StorageCACert)
	if len(caCertPEM) > 0 {
		config.caCertPEM = []byte(caCertPEM)
	}

	caKeyPEM := agentConfig.Value(StorageCAKey)
	if len(caKeyPEM) > 0 {
		config.caKeyPEM = []byte(caKeyPEM)
	}

	hostnames := agentConfig.Value(StorageHostnames)
	if len(hostnames) > 0 {
		err := goyaml.Unmarshal([]byte(hostnames), &config.hostnames)
		if err != nil {
			return nil, err
		}
	}

	return config, nil
}
