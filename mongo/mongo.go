// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

const (
	// This is NUMACTL package name for apt-get
	numaCtlPkg = "numactl"
)

var (
	logger = loggo.GetLogger("juju.mongo")
)

// SSLInfo holds all the SSL-related mongod server params.
type SSLInfo struct {
	// Cert is the certificate.
	Cert string

	// PrivateKey is the certificate's private key.
	PrivateKey string

	// CAPrivateKey is the CA certificate's private key.
	CAPrivateKey string

	// SharedSecret is a secret shared between mongo servers.
	SharedSecret string
}

// CertKey returns the combined SSL cert and key to use for mongo.
func (si SSLInfo) CertKey() string {
	return si.Cert + "\n" + si.PrivateKey
}

// EnsureServerParams is a parameter struct for EnsureServer.
type EnsureServerParams struct {
	SSLInfo

	// APIPort is the port to connect to the api server.
	APIPort int

	// StatePort is the port to connect to the mongo server.
	StatePort int

	// TODO(ericsnow) Can SystemIdentity be dropped?

	// SystemIdentity is the identity of the system.
	SystemIdentity string

	// DataDir is the machine agent data directory.
	DataDir string

	// Namespace is the machine agent's namespace, which is used to
	// generate a unique service name for Mongo.
	Namespace string

	// OplogSize is the size of the Mongo oplog.
	// If this is zero, then EnsureServer will
	// calculate a default size according to the
	// algorithm defined in Mongo.
	OplogSize int

	// SetNumaControlPolicy preference - whether the user
	// wants to set the numa control policy when starting mongo.
	SetNumaControlPolicy bool
}

// DBDir returns the directory where mongod data should be stored.
func (esp EnsureServerParams) DBDir() string {
	return filepath.Join(esp.DataDir, "db")
}

// EnsureServer ensures that the MongoDB server is installed,
// configured, and ready to run.
//
// This method will remove old versions of the mongo init service as necessary
// before installing the new version.
//
// The namespace is a unique identifier to prevent multiple instances of mongo
// on this machine from colliding. This should be empty unless using
// the local provider.
func EnsureServer(args EnsureServerParams) error {
	logger.Infof(
		"Ensuring mongo server is running; data directory %s; port %d",
		args.DataDir, args.StatePort,
	)

	// Make sure the DB dir exists *before* we do anything else.
	dbDir := DBDir(args.DataDir)
	if err := makeDBDir(dbDir); err != nil {
		return errors.Annotate(err, "cannot create mongo database directory")
	}

	// Extrapolate data from the args.
	// TODO(ericsnow) If we passed args.DataDir here instead of dbDir
	// then we could move creating the DB dir into resetAndInstall.
	oplogSizeMB := args.OplogSize
	if oplogSizeMB == 0 {
		defaultSize, err := defaultOplogSize(dbDir)
		if err != nil {
			return errors.Trace(err)
		}
		oplogSizeMB = defaultSize
	}

	// TODO(ericsnow) Why do we apt-get install mongo here (before
	// checking if we're already running it), as well as the svc.Manage
	// call in svc.startIfInstalled, instead of in resetAndInstall?

	// Make sure the mongod package is installed.
	if err := aptGetInstallMongod(args.SetNumaControlPolicy); err != nil {
		return errors.Annotate(err, "cannot install mongod")
	}

	// Prep the (upstart) service.
	spec := ServiceSpec{
		DBDir:       dbDir,
		DataDir:     args.DataDir,
		Port:        args.StatePort,
		OplogSizeMB: oplogSizeMB,
		WantNumaCtl: args.SetNumaControlPolicy,
	}
	if err := spec.ApplyDefaults(); err != nil {
		return errors.Trace(err)
	}
	svc, err := spec.NewService(args.Namespace)
	if err != nil {
		return errors.Trace(err)
	}

	// Start the service if it's already enabled.
	running, err := svc.startIfInstalled()
	if err != nil {
		return errors.Trace(err)
	}
	if running {
		return nil
	}

	// Service not installed, so reset the configs and install it.
	err = resetAndInstall(svc, args, oplogSizeMB)
	return errors.Trace(err)
}

func resetAndInstall(svc *Service, args EnsureServerParams, oplogSizeMB int) error {
	dbDir := DBDir(args.DataDir)

	// Try stopping the service, just in case.
	if err := svc.Stop(); err != nil && !errors.IsNotFound(err) {
		return errors.Errorf("failed to stop mongo: %v", err)
	}

	// Re-write the SSL-related files (e.g. cert file).
	if err := writeSSL(args.DataDir, args.SSLInfo); err != nil {
		return errors.Trace(err)
	}

	// Disable the default mongodb installed by the mongodb-server package.
	// Only do this if the file doesn't exist already, so users can run
	// their own mongodb server if they wish to.
	_, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		if err := writeConf("ENABLE_MONGODB=no"); err != nil {
			return errors.Trace(err)
		}
	} else if err != nil {
		return errors.Trace(err)
	}

	// Set up other dirs and files.
	if err := makeJournalDirs(dbDir); err != nil {
		return errors.Errorf("error creating journal directories: %v", err)
	}
	if err := preallocOplog(dbDir, oplogSizeMB); err != nil {
		return errors.Errorf("error creating oplog files: %v", err)
	}

	// Enable and start the service.
	err = installService(svc)
	return errors.Trace(err)
}

var installService = func(svc *Service) error {
	if err := svc.Enable(); err != nil {
		return errors.Trace(err)
	}
	if err := svc.Start(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func logVersion(mongoPath string) {
	cmd := exec.Command(mongoPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Infof("failed to read the output from %s --version: %v", mongoPath, err)
		return
	}
	logger.Debugf("using mongod: %s --version: %q", mongoPath, output)
}
