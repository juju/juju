// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/network"
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
	// reach, down to 256MB.
	LowCacheSize = 0.25

	// flagMarker is an in-line comment for bash. If it somehow makes its way onto
	// the command line, it will be ignored. See https://stackoverflow.com/a/1456019/395287
	flagMarker = "`#flag: true` \\"

	dataPathForJujuDbSnap = "/var/snap/juju-db/common"

	// FileNameDBSSLKey is the file name of db ssl key file name.
	FileNameDBSSLKey = "server.pem"
)

// See https://docs.mongodb.com/manual/reference/ulimit/.
var mongoULimits = map[string]string{
	"fsize":   "unlimited", // file size
	"cpu":     "unlimited", // cpu time
	"as":      "unlimited", // virtual memory size
	"memlock": "unlimited", // locked-in-memory size
	"nofile":  "64000",     // open files
	"nproc":   "64000",     // processes/threads
}

func sslKeyPath(dataDir string) string {
	return filepath.Join(dataDir, FileNameDBSSLKey)
}

func sharedSecretPath(dataDir string) string {
	return filepath.Join(dataDir, SharedSecretFile)
}

func logPath(dataDir string) string {
	return filepath.Join(dataDir, "logs", "mongodb.log")
}

func configPath(dataDir string) string {
	return filepath.Join(dataDir, "juju-db.config")
}

// ConfigArgs holds the attributes of a service configuration for mongo.
type ConfigArgs struct {
	Clock clock.Clock

	DataDir    string
	DBDir      string
	ReplicaSet string

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
	TLSOnNormalPorts bool
	TLSMode          string

	// logging
	Syslog    bool
	LogAppend bool
	LogPath   string

	// db kernel
	MemoryProfile         MemoryProfile
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
	pathArgs := set.NewStrings("dbpath", "logpath", "tlsCertificateKeyFile", "keyFile")
	command := make([]string, 0, len(conf))
	var keys []string
	for k := range conf {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := conf[key]
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
		if strings.HasPrefix(key, "tlsCertificateKeyFilePassword") {
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
	if mongoArgs.TLSMode != "" {
		result["tlsMode"] = mongoArgs.TLSMode
	}
	if mongoArgs.LogAppend {
		result["logappend"] = flagMarker
	}

	if mongoArgs.TLSOnNormalPorts {
		result["tlsOnNormalPorts"] = flagMarker
	}

	// authn
	if mongoArgs.PEMKeyFile != "" {
		result["tlsCertificateKeyFile"] = utils.ShQuote(mongoArgs.PEMKeyFile)
		//--tlsCertificateKeyFilePassword must be concatenated to the equals sign (lp:1581284)
		pemPassword := mongoArgs.PEMKeyPassword
		if pemPassword == "" {
			pemPassword = "ignored"
		}
		result["tlsCertificateKeyFilePassword="+pemPassword] = flagMarker
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
	result["journal"] = flagMarker
	if mongoArgs.OplogSizeMB != 0 {
		result["oplogSize"] = strconv.Itoa(mongoArgs.OplogSizeMB)
	}

	result["storageEngine"] = string(WiredTiger)
	if mongoArgs.WiredTigerCacheSizeGB > 0.0 {
		result["wiredTigerCacheSizeGB"] = fmt.Sprint(mongoArgs.WiredTigerCacheSizeGB)
	}

	// misc
	if mongoArgs.Quiet {
		result["quiet"] = flagMarker
	}

	return result
}

func (mongoArgs *ConfigArgs) writeConfig(path string) error {
	generatedAt := mongoArgs.Clock.Now().UTC().Format(time.RFC822)
	configPrologue := fmt.Sprintf(`
# WARNING
# autogenerated by juju on %v
# manual changes to this file are likely be overwritten
`[1:], generatedAt)
	configBody := mongoArgs.asMap().asMongoDbConfigurationFileFormat()
	config := []byte(configPrologue + configBody)

	err := utils.AtomicWriteFile(path, config, 0644)
	if err != nil {
		return errors.Annotate(err, fmt.Sprintf("writingconfig to %s", path))
	}

	return nil
}

// Override for testing.
var supportsIPv6 = network.SupportsIPv6

// newMongoDBArgsWithDefaults returns *mongoDbConfigArgs
// under the assumption that MongoDB 3.4 or later is running.
func generateConfig(oplogSizeMB int, args EnsureServerParams) *ConfigArgs {
	useLowMemory := args.MemoryProfile == MemoryProfileLow

	mongoArgs := &ConfigArgs{
		Clock:         clock.WallClock,
		DataDir:       args.DataDir,
		DBDir:         dbDir(args.DataDir),
		LogPath:       logPath(args.DataDir),
		Port:          args.StatePort,
		OplogSizeMB:   oplogSizeMB,
		IPv6:          supportsIPv6(),
		MemoryProfile: args.MemoryProfile,
		// Switch from syslog to appending to dataDir, because snaps don't
		// have the same permissions.
		Syslog:           false,
		LogAppend:        true,
		Quiet:            true,
		ReplicaSet:       ReplicaSetName,
		AuthKeyFile:      sharedSecretPath(args.DataDir),
		PEMKeyFile:       sslKeyPath(args.DataDir),
		PEMKeyPassword:   "ignored", // used as boilerplate later
		TLSOnNormalPorts: false,
		TLSMode:          "requireTLS",
		BindToAllIP:      true, // TODO(tsm): disable when not needed
		//BindIP:         "127.0.0.1", // TODO(tsm): use machine's actual IP address via dialInfo
	}

	if useLowMemory {
		mongoArgs.WiredTigerCacheSizeGB = LowCacheSize
	}
	return mongoArgs
}
