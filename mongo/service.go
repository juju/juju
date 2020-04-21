// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/network"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/snap"
)

const (
	// ServiceName is the name of the service that Juju's mongod instance
	// will be named.
	ServiceName    = "juju-db"
	serviceTimeout = 300 // 5 minutes

	// SharedSecretFile is the name of the Mongo shared secret file
	// located within the Juju data directory.
	SharedSecretFile = "shared-secret"

	// ReplicaSetName is the name of the replica set that juju uses for its
	// controllers.
	ReplicaSetName = "juju"

	// LowCacheSize expressed in GB sets the max value Mongo WiredTiger cache can
	// reach.
	LowCacheSize = 1

	// Mongo34LowCacheSize changed to being a float, and allows you to specify down to 256MB
	Mongo34LowCacheSize = 0.25

	// flagMarker is an in-line comment for bash. If it somehow makes its way onto
	// the command line, it will be ignored. See https://stackoverflow.com/a/1456019/395287
	flagMarker = "`#flag: true` \\"

	dataPathForJujuDbSnap = "/var/snap/juju-db/common"

	// mongoLogPath is used as a fallback location when syslog is not enabled
	mongoLogPath = "/var/log/mongodb"

	// FileNameDBSSLKey is the file name of db ssl key file name.
	FileNameDBSSLKey = "server.pem"
)

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

// mongoService is a slimmed-down version of the service.Service interface.
type mongoService interface {
	Exists() (bool, error)
	Installed() (bool, error)
	Running() (bool, error)
	service.ServiceActions
}

var newService = func(name string, usingSnap bool, conf common.Conf) (mongoService, error) {
	if usingSnap {
		return snap.NewServiceFromName(name, conf)
	}
	return service.DiscoverService(name, conf)
}

var discoverService = func(name string) (mongoService, error) {
	return newService(name, false, common.Conf{})
}

// IsServiceInstalled returns whether the MongoDB init service
// configuration is present.
var IsServiceInstalled = isServiceInstalled

func isServiceInstalled() (bool, error) {
	svc, err := discoverService(ServiceName)
	if err != nil {
		return false, errors.Trace(err)
	}
	return svc.Installed()
}

// RemoveService removes the mongoDB init service from this machine.
func RemoveService() error {
	svc, err := discoverService(ServiceName)
	if err != nil {
		return errors.Trace(err)
	}
	if err := svc.Stop(); err != nil {
		return errors.Trace(err)
	}
	if err := svc.Remove(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// StopService will stop mongodb service.
func StopService() error {
	svc, err := discoverService(ServiceName)
	if err != nil {
		return errors.Trace(err)
	}
	return svc.Stop()
}

// StartService will start mongodb service.
func StartService() error {
	svc, err := discoverService(ServiceName)
	if err != nil {
		return errors.Trace(err)
	}
	return svc.Start()
}

// ReStartService will stop and then start mongodb service.
func ReStartService() error {
	// TODO(tsm): refactor to make use of service.RestartableService
	svc, err := discoverService(ServiceName)
	if err != nil {
		return errors.Trace(err)
	}
	return restartService(svc)
}

func restartService(svc mongoService) error {
	// TODO(tsm): refactor to make use of service.RestartableService
	if err := svc.Stop(); err != nil {
		return errors.Trace(err)
	}
	if err := svc.Start(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func sslKeyPath(dataDir string) string {
	return filepath.Join(dataDir, FileNameDBSSLKey)
}

func sharedSecretPath(dataDir string) string {
	return filepath.Join(dataDir, SharedSecretFile)
}

func logPath(dataDir string, usingMongoFromSnap bool) string {
	if usingMongoFromSnap {
		return filepath.Join(dataDir, "logs", "mongodb.log")
	}
	return mongoLogPath
}

func configPath(dataDir string) string {
	return filepath.Join(dataDir, "juju-db.config")
}

// ConfigArgs holds the attributes of a service configuration for mongo.
type ConfigArgs struct {
	DataDir    string
	DBDir      string
	MongoPath  string
	ReplicaSet string
	Version    Version

	// connection params
	BindIP      string
	BindToAllIP bool
	Port        int
	OplogSizeMB int

	// auth
	AuthKeyFile    string
	PEMKeyFile     string
	PEMKeyPassword string

	// network params
	IPv6             bool
	SSLOnNormalPorts bool
	SSLMode          string

	// logging
	Syslog    bool
	LogAppend bool
	LogPath   string

	// db kernel
	WantNUMACtl           bool
	MemoryProfile         MemoryProfile
	Journal               bool
	NoPreAlloc            bool
	SmallFiles            bool
	WiredTigerCacheSizeGB float32

	// misc
	Quiet bool
}

type configArgsConverter map[string]string

// The generated command line arguments need to be in a deterministic order.
func (conf configArgsConverter) asCommandLineArguments() string {
	keys := make([]string, 0, len(conf))
	for key := range conf {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	command := make([]string, 0, len(conf)*2)
	for _, key := range keys {
		value := conf[key]
		if len(key) >= 2 {
			key = "--" + key
		} else if len(key) == 1 {
			key = "-" + key
		} else {
			continue // impossible?
		}
		command = append(command, key)

		if value == flagMarker {
			continue
		}
		command = append(command, value)
	}

	return strings.Join(command, " ")
}

func (conf configArgsConverter) asMongoDbConfigurationFileFormat() string {
	pathArgs := set.NewStrings("dbpath", "logpath", "sslPEMKeyFile", "keyFile")
	command := make([]string, 0, len(conf))
	for key, value := range conf {
		if len(key) == 0 {
			continue
		}
		if pathArgs.Contains(key) {
			value = strings.Trim(value, " '")
		}
		if value == flagMarker {
			value = "true"
		}
		line := fmt.Sprintf("%s = %s", key, value)
		if strings.HasPrefix(key, "sslPEMKeyPassword") {
			line = key
		}
		command = append(command, line)
	}

	return strings.Join(command, "\n")
}

func (mongoArgs *ConfigArgs) asMap() configArgsConverter {
	result := configArgsConverter{}
	result["replSet"] = mongoArgs.ReplicaSet
	result["dbpath"] = utils.ShQuote(mongoArgs.DBDir)

	if mongoArgs.LogPath != "" {
		result["logpath"] = utils.ShQuote(mongoArgs.LogPath)
	}

	if mongoArgs.BindIP != "" {
		result["bind_ip"] = mongoArgs.BindIP
	}
	if mongoArgs.Port != 0 {
		result["port"] = strconv.Itoa(mongoArgs.Port)
	}
	if mongoArgs.IPv6 {
		result["ipv6"] = flagMarker
	}
	if mongoArgs.BindToAllIP {
		result["bind_ip_all"] = flagMarker
	}
	if mongoArgs.SSLMode != "" {
		result["sslMode"] = mongoArgs.SSLMode
	}
	if mongoArgs.LogAppend {
		result["logappend"] = flagMarker
	}

	if mongoArgs.SSLOnNormalPorts {
		result["sslOnNormalPorts"] = flagMarker
	}

	// authn
	if mongoArgs.PEMKeyFile != "" {
		result["sslPEMKeyFile"] = utils.ShQuote(mongoArgs.PEMKeyFile)
		//--sslPEMKeyPassword must be concatenated to the equals sign (lp:1581284)
		pemPassword := mongoArgs.PEMKeyPassword
		if pemPassword == "" {
			pemPassword = "ignored"
		}
		result["sslPEMKeyPassword="+pemPassword] = flagMarker
	}

	if mongoArgs.AuthKeyFile != "" {
		result["auth"] = flagMarker
		result["keyFile"] = utils.ShQuote(mongoArgs.AuthKeyFile)
	} else {
		logger.Warningf("configuring mongod  with --noauth flag enabled")
		result["noauth"] = flagMarker
	}

	// ops config
	if mongoArgs.Syslog {
		result["syslog"] = flagMarker
	}
	if mongoArgs.Journal {
		result["journal"] = flagMarker
	}
	if mongoArgs.OplogSizeMB != 0 {
		result["oplogSize"] = strconv.Itoa(mongoArgs.OplogSizeMB)
	}
	if mongoArgs.NoPreAlloc {
		result["noprealloc"] = flagMarker
	}
	if mongoArgs.SmallFiles {
		result["smallfiles"] = flagMarker
	}

	// storageEngine is an unsupported argument for mongo 2.x
	if mongoArgs.Version.Major >= 3 && mongoArgs.Version.StorageEngine != "" {
		result["storageEngine"] = string(mongoArgs.Version.StorageEngine)
	}
	if mongoArgs.WiredTigerCacheSizeGB > 0.0 {
		result["wiredTigerCacheSizeGB"] = fmt.Sprint(mongoArgs.WiredTigerCacheSizeGB)
	}

	// misc
	if mongoArgs.Quiet {
		result["quiet"] = flagMarker
	}

	return result
}

func (mongoArgs *ConfigArgs) asService(usingMongoFromSnap bool) (mongoService, error) {
	return newService(ServiceName, usingMongoFromSnap, mongoArgs.asServiceConf(usingMongoFromSnap))
}

// asServiceConf returns the init system config for the mongo state service.
func (mongoArgs *ConfigArgs) asServiceConf(usingMongoFromSnap bool) common.Conf {
	// See https://docs.mongodb.com/manual/reference/ulimit/.
	limits := map[string]string{
		"fsize":   "unlimited", // file size
		"cpu":     "unlimited", // cpu time
		"as":      "unlimited", // virtual memory size
		"memlock": "unlimited", // locked-in-memory size
		"nofile":  "64000",     // open files
		"nproc":   "64000",     // processes/threads
	}
	conf := common.Conf{
		Desc:        "juju state database",
		Limit:       limits,
		Timeout:     serviceTimeout,
		ExecStart:   mongoArgs.startCommand(usingMongoFromSnap),
		ExtraScript: mongoArgs.extraScript(usingMongoFromSnap),
	}
	return conf
}

func (mongoArgs *ConfigArgs) asMongoDbConfigurationFileFormat() string {
	return mongoArgs.asMap().asMongoDbConfigurationFileFormat()
}

func (mongoArgs *ConfigArgs) asCommandLineArguments() string {
	return mongoArgs.MongoPath + " " + mongoArgs.asMap().asCommandLineArguments()
}

func (mongoArgs *ConfigArgs) startCommand(usingMongoFromSnap bool) string {
	if usingMongoFromSnap {
		// TODO(tsm): work out how to bridge mongoService and service.Service
		// to access the StartCommands method, rather than duplicating code
		return snap.Command + " start --enable " + ServiceName
	}

	cmd := ""
	if mongoArgs.WantNUMACtl {
		cmd = fmt.Sprintf(numaCtlWrap, multinodeVarName) + " "
	}
	return cmd + mongoArgs.asCommandLineArguments()
}

func (mongoArgs *ConfigArgs) extraScript(usingMongoFromSnap bool) string {
	if usingMongoFromSnap {
		return ""
	}

	cmd := ""
	if mongoArgs.WantNUMACtl {
		cmd = fmt.Sprintf(detectMultiNodeScript, multinodeVarName, multinodeVarName)
	}
	return cmd
}

func (mongoArgs *ConfigArgs) writeConfig(path string) error {
	generatedAt := time.Now().String()
	configPrologue := fmt.Sprintf(`
# WARNING
# autogenerated by juju on %v
# manual changes to this file are likely be overwritten
`[1:], generatedAt)
	configBody := mongoArgs.asMongoDbConfigurationFileFormat()
	config := []byte(configPrologue + configBody)

	err := utils.AtomicWriteFile(path, config, 0644)
	if err != nil {
		return errors.Annotate(err, fmt.Sprintf("writingconfig to %s", path))
	}

	return nil
}

// newMongoDBArgsWithDefaults returns *mongoDbConfigArgs
// under the assumption that MongoDB 3.4 or later is running.
func generateConfig(mongoPath string, oplogSizeMB int, version Version, usingMongoFromSnap bool, args EnsureServerParams) *ConfigArgs {
	usingWiredTiger := version.StorageEngine == WiredTiger
	usingMongo2 := version.Major == 2
	usingMongo4orAbove := version.Major > 3
	usingMongo36orAbove := usingMongo4orAbove || (version.Major == 3 && version.Minor >= 6)
	usingMongo34orAbove := usingMongo36orAbove || (version.Major == 3 && version.Minor >= 4)
	useLowMemory := args.MemoryProfile == MemoryProfileLow

	mongoArgs := &ConfigArgs{
		DataDir:          args.DataDir,
		DBDir:            DbDir(args.DataDir),
		MongoPath:        mongoPath,
		Port:             args.StatePort,
		OplogSizeMB:      oplogSizeMB,
		WantNUMACtl:      args.SetNUMAControlPolicy,
		Version:          version,
		IPv6:             network.SupportsIPv6(),
		MemoryProfile:    args.MemoryProfile,
		Syslog:           true,
		Journal:          true,
		Quiet:            true,
		ReplicaSet:       ReplicaSetName,
		AuthKeyFile:      sharedSecretPath(args.DataDir),
		PEMKeyFile:       sslKeyPath(args.DataDir),
		PEMKeyPassword:   "ignored", // used as boilerplate later
		SSLOnNormalPorts: false,
		//BindIP:                "127.0.0.1", // TODO(tsm): use machine's actual IP address via dialInfo
	}

	if useLowMemory && usingWiredTiger {
		if usingMongo34orAbove {
			// Mongo 3.4 introduced the ability to have fractional GB cache size.
			mongoArgs.WiredTigerCacheSizeGB = Mongo34LowCacheSize
		} else {
			mongoArgs.WiredTigerCacheSizeGB = LowCacheSize
		}
	}
	if !usingWiredTiger {
		mongoArgs.NoPreAlloc = true
		mongoArgs.SmallFiles = true
	}

	if usingMongo36orAbove {
		mongoArgs.BindToAllIP = true
	}

	if usingMongo2 {
		mongoArgs.SSLOnNormalPorts = true
	} else {
		mongoArgs.SSLMode = "requireSSL"
	}

	if usingMongoFromSnap {
		// Switch from syslog to appending to dataDir, because snaps don't
		// have the same permissions.
		mongoArgs.Syslog = false
		mongoArgs.LogAppend = true
		mongoArgs.LogPath = logPath(args.DataDir, true)
		mongoArgs.BindToAllIP = true // TODO(tsm): disable when not needed
	}

	return mongoArgs
}

// newConf returns the init system config for the mongo state service.
func newConf(args *ConfigArgs) common.Conf {
	usingJujuDBSnap := args.DataDir == dataPathForJujuDbSnap
	return args.asServiceConf(usingJujuDBSnap)
}

func ensureDirectoriesMade(dataDir string) error {
	dbDir := DbDir(dataDir)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return errors.Annotate(err, "cannot create mongo database directory")
	}
	return nil
}

// EnsureServiceInstalled is a convenience method to [re]create
// the mongo service. It assumes that the necessary packages are
// already installed and that the file system includes secrets at dataDir.
func EnsureServiceInstalled(dataDir string, statePort int, oplogSizeMB int, setNUMAControlPolicy bool, version Version, auth bool, memProfile MemoryProfile) error {
	// TODO(tsm): delete EnsureServiceInstalled and use EnsureServer for upgrades
	//            once upgrade_mongo is removed.
	err := ensureDirectoriesMade(dataDir)
	if err != nil {
		return errors.Trace(err)
	}

	usingMongoFromSnap := dataDir == dataPathForJujuDbSnap

	mongoPath, err := Path(version)
	if err != nil {
		return errors.NewNotFound(err, "unable to find path to mongod")
	}

	mongoArgs := generateConfig(
		mongoPath,
		oplogSizeMB,
		version,
		usingMongoFromSnap,
		EnsureServerParams{
			DataDir:              dataDir,
			StatePort:            statePort,
			SetNUMAControlPolicy: setNUMAControlPolicy,
			MemoryProfile:        memProfile,
		},
	)

	if !auth {
		mongoArgs.AuthKeyFile = ""
	}

	service, err := mongoArgs.asService(usingMongoFromSnap)
	if err != nil {
		return errors.Trace(err)
	}

	err = service.Install()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
