// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/juju/errors"

	"github.com/juju/juju/network"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

// TODO(juju4): remove support for upgrading from older mongos

// maybeUseLegacyMongo returns nil if there's a juju that's running
// on a mongo that's not from the juju-db snap.
// This is used to preserve the setup up older versions being upgraded.
func maybeUseLegacyMongo(args EnsureServerParams, search SearchTools) error {
	if args.DataDir == "" {
		args.DataDir = "/var/lib/juju"
	}
	mongoPath, mongodVersion, err := findLegacyMongo(search)
	if err != nil {
		if errors.IsNotFound(err) {
			// As a safety check, if there's no mongo installed but there's
			// database files present, that's an issue since we can only use
			// an older mongo to read them..
			if search.Exists(dbDir(args.DataDir)) {
				return errors.New("mongo database files exist but no mongo is installed")
			}
		}
		return errors.Trace(err)
	}
	// Coming from an early trusty install is not supported since
	// we now only support wired tiger.
	if mongodVersion.Major == 2 {
		return errors.NotSupportedf("mongo %v", mongodVersion)
	}
	logVersion(mongoPath)

	// We have the mongo binary and database files; the following
	// code ensures the systemd service is configured.

	oplogSizeMB := args.OplogSize
	if oplogSizeMB == 0 {
		oplogSizeMB, err = defaultOplogSize(dbDir(args.DataDir))
		if err != nil {
			return errors.Trace(err)
		}
	}
	mongoArgs := generateLegacyConfig(mongoPath, oplogSizeMB, mongodVersion, args)

	svc, err := mongoArgs.asService()
	if err != nil {
		return errors.Trace(err)
	}

	installed, err := svc.Installed()
	if err != nil {
		return errors.Trace(err)
	}
	if !installed {
		return errors.NotFoundf("service " + ServiceName)
	}

	// Exists() does a check against the contents of the service config file.
	// The return value is true iff the content is the same.
	exists, err := svc.Exists()
	if err != nil {
		return errors.Trace(err)
	}
	if exists {
		logger.Debugf("mongo exists as expected")
		running, err := svc.Running()
		if err != nil {
			return errors.Trace(err)
		}

		if !running {
			return errors.Trace(svc.Start())
		}
		return nil
	}
	logger.Debugf("updating mongo service configuration")

	// We want to write or rewrite the contents of the service.
	// Stop is a no-op if the service doesn't exist or isn't running.
	if err := svc.Stop(); err != nil {
		return errors.Annotatef(err, "failed to stop mongo")
	}
	if err := service.InstallAndStart(svc); err != nil {
		return errors.Trace(err)
	}
	return nil
}

const (
	jujuMongod24Path = "/usr/lib/juju/bin/mongod"
	jujuMongod32Path = "/usr/lib/juju/mongo3.2/bin/mongod"
	mongodSystemPath = "/usr/bin/mongod"
)

func findLegacyMongo(search SearchTools) (string, version, error) {
	// In Bionic and beyond (and early trusty) we just use the system mongo.
	// We only use the system mongo if it is at least Mongo 3.4
	if search.Exists(mongodSystemPath) {
		// We found Mongo in the system directory, check to see if the version is valid
		if v, err := findVersion(search, mongodSystemPath); err != nil {
			logger.Warningf("system mongo %q found, but ignoring error trying to get version: %v",
				mongodSystemPath, err)
		} else if v.Major > 3 || (v.Major == 3 && v.Minor >= 4) {
			// We only support mongo 3.4 and newer from the system
			return mongodSystemPath, v, nil
		}
	}
	// the system mongo is either too old, or not valid, keep trying
	if search.Exists(jujuMongod32Path) {
		// juju-mongod32 is available, check its version as well. Mostly just as a reporting convenience
		// Do we want to use it even if we can't deal with --version?
		v, err := findVersion(search, jujuMongod32Path)
		if err != nil {
			logger.Warningf("juju-mongodb3.2 %q found, but ignoring error trying to get version: %v",
				jujuMongod32Path, err)
			v = version{Major: 3, Minor: 2}
		}
		return jujuMongod32Path, v, nil
	}
	if search.Exists(jujuMongod24Path) {
		return jujuMongod24Path, version{Major: 2, Minor: 4}, nil
	}
	return "", version{}, errors.NotFoundf("could not find a viable 'mongod'")
}

// all mongo versions start with "db version v" and then the version is a X.Y.Z-extra
// we don't really care about the 'extra' portion of it, so we just track the rest.
var mongoVersionRegex = regexp.MustCompile(`^db version v(\d{1,9})\.(\d{1,9}).(\d{1,9})([.-].*)?`)

// parseMongoVersion parses the output from "mongod --version" and returns a Version struct
func parseMongoVersion(versionInfo string) (version, error) {
	m := mongoVersionRegex.FindStringSubmatch(versionInfo)
	if m == nil {
		return version{}, errors.Errorf("'mongod --version' reported:\n%s", versionInfo)
	}
	if len(m) < 4 {
		return version{}, errors.Errorf("did not find enough version parts in:\n%s", versionInfo)
	}
	var v version
	var err error
	// Index '[0]' is the full matched string,
	// [1] is the Major
	// [2] is the Minor
	// [3] is the Point
	if v.Major, err = strconv.Atoi(m[1]); err != nil {
		return version{}, errors.Annotatef(err, "invalid major version: %q", versionInfo)
	}
	if v.Minor, err = strconv.Atoi(m[2]); err != nil {
		return version{}, errors.Annotatef(err, "invalid minor version: %q", versionInfo)
	}
	if v.Point, err = strconv.Atoi(m[3]); err != nil {
		return version{}, errors.Annotatef(err, "invalid point version: %q", versionInfo)
	}
	return v, nil
}

func findVersion(search SearchTools, path string) (version, error) {
	out, err := search.GetCommandOutput(path, "--version")
	if err != nil {
		return version{}, errors.Trace(err)
	}
	v, err := parseMongoVersion(out)
	if err != nil {
		return version{}, errors.Trace(err)
	}
	return v, nil
}

type version struct {
	Major int
	Minor int
	Point int
}

type legacyConfigArgs struct {
	ConfigArgs

	mongoPath   string
	wantNUMACtl bool
	version     version
}

func generateLegacyConfig(mongoPath string, oplogSizeMB int, version version, args EnsureServerParams) *legacyConfigArgs {
	usingMongo4orAbove := version.Major > 3
	usingMongo36orAbove := usingMongo4orAbove || (version.Major == 3 && version.Minor >= 6)
	usingMongo34orAbove := usingMongo36orAbove || (version.Major == 3 && version.Minor >= 4)
	useLowMemory := args.MemoryProfile == MemoryProfileLow

	mongoArgs := &legacyConfigArgs{
		mongoPath:   mongoPath,
		wantNUMACtl: args.SetNUMAControlPolicy,
		version:     version,
		ConfigArgs: ConfigArgs{
			DataDir:          args.DataDir,
			DBDir:            dbDir(args.DataDir),
			Port:             args.StatePort,
			OplogSizeMB:      oplogSizeMB,
			IPv6:             network.SupportsIPv6(),
			MemoryProfile:    args.MemoryProfile,
			Syslog:           true,
			Quiet:            true,
			ReplicaSet:       ReplicaSetName,
			AuthKeyFile:      sharedSecretPath(args.DataDir),
			PEMKeyFile:       sslKeyPath(args.DataDir),
			PEMKeyPassword:   "ignored", // used as boilerplate later
			SSLOnNormalPorts: false,
			SSLMode:          "requireSSL",
		},
	}

	if useLowMemory {
		if usingMongo34orAbove {
			// Mongo 3.4 introduced the ability to have fractional GB cache size.
			mongoArgs.WiredTigerCacheSizeGB = LowCacheSize
		} else {
			mongoArgs.WiredTigerCacheSizeGB = 1
		}
	}

	if usingMongo36orAbove {
		mongoArgs.BindToAllIP = true
	}
	return mongoArgs
}

func (mongoArgs *legacyConfigArgs) asService() (mongoService, error) {
	return newService(ServiceName, common.Conf{
		Desc:        "juju state database",
		Limit:       mongoULimits,
		Timeout:     serviceTimeout,
		ExecStart:   mongoArgs.startCommand(),
		ExtraScript: mongoArgs.extraScript(),
	})
}

func (mongoArgs *legacyConfigArgs) asMongoDbConfigurationFileFormat() string {
	return mongoArgs.asMap().asMongoDbConfigurationFileFormat()
}

func (mongoArgs *legacyConfigArgs) asCommandLineArguments() string {
	return mongoArgs.mongoPath + " " + mongoArgs.asMap().asCommandLineArguments()
}

var (
	// This is the name of an environment variable that we use in the
	// init system conf file when mongo NUMA support is used.
	multinodeVarName = "MULTI_NODE"
	// This value will be used to wrap desired mongo cmd in numactl if wanted/needed
	numaCtlWrap = "$%v"
	// Extra shell script fragment for init script template.
	// This determines if we are dealing with multi-node environment
	detectMultiNodeScript = `%v=""
if [ $(find /sys/devices/system/node/ -maxdepth 1 -mindepth 1 -type d -name node\* | wc -l ) -gt 1 ]
then
    %v=" numactl --interleave=all "
    # Ensure sysctl turns off zone_reclaim_mode if not already set
    (grep -q vm.zone_reclaim_mode /etc/sysctl.conf || echo vm.zone_reclaim_mode = 0 >> /etc/sysctl.conf) && sysctl -p
fi
`
)

func (mongoArgs *legacyConfigArgs) startCommand() string {
	cmd := ""
	if mongoArgs.wantNUMACtl {
		cmd = fmt.Sprintf(numaCtlWrap, multinodeVarName) + " "
	}
	return cmd + mongoArgs.asCommandLineArguments()
}

func (mongoArgs *legacyConfigArgs) extraScript() string {
	cmd := ""
	if mongoArgs.wantNUMACtl {
		cmd = fmt.Sprintf(detectMultiNodeScript, multinodeVarName, multinodeVarName)
	}
	return cmd
}

// mongoService is a slimmed-down version of the service.Service interface.
type mongoService interface {
	Exists() (bool, error)
	Installed() (bool, error)
	Running() (bool, error)
	service.ServiceActions
}

var newService = func(name string, conf common.Conf) (mongoService, error) {
	return service.DiscoverService(name, conf)
}
