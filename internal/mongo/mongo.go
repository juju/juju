// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"context"
)

// JujuDbSnapMongodPath is the path that the juju-db snap
// makes mongod available at
var JujuDbSnapMongodPath = "/snap/bin/juju-db.mongod"

// EnsureServerParams is a parameter struct for EnsureServer.
type EnsureServerParams struct {
	// APIPort is the port to connect to the api server.
	APIPort int

	// Cert is the certificate.
	Cert string

	// PrivateKey is the certificate's private key.
	PrivateKey string

	// CAPrivateKey is the CA certificate's private key.
	CAPrivateKey string

	// SystemIdentity is the identity of the system.
	SystemIdentity string

	// MongoDataDir is the machine agent mongo data directory.
	MongoDataDir string

	// JujuDataDir is the directory where juju data is stored.
	JujuDataDir string

	// ConfigDir is where mongo config goes.
	ConfigDir string

	// Namespace is the machine agent's namespace, which is used to
	// generate a unique service name for Mongo.
	Namespace string

	// OplogSize is the size of the Mongo oplog.
	// If this is zero, then EnsureServer will
	// calculate a default size according to the
	// algorithm defined in Mongo.
	OplogSize int

	// SetNUMAControlPolicy preference - whether the user
	// wants to set the numa control policy when starting mongo.
	SetNUMAControlPolicy bool
}

// EnsureServerInstalled ensures that the MongoDB server is installed,
// configured, and ready to run.
func EnsureServerInstalled(ctx context.Context, args EnsureServerParams) error {
	return nil
}
