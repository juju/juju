// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The feature package defines the names of the current feature flags.
package feature

import (
	"github.com/juju/utils/featureflag"
)

// TODO (anastasiamac 2015-03-02)
// Features that have commands that can be blocked,
// command list for "juju block" and "juju unblock"
// needs to be maintained until we can dynamically discover
// these commands.

// JES stands for Juju Environment Server and controls access
// to the apiserver endpoints, api client and CLI commands.
// It also guards the writing of the new
// $JUJU_HOME/environments/cache.yaml file.  If this flag is
// set, new environments will be written to the cache file
// rather than a JENV file. As JENV files are updated, they
// are migrated to the cache file and the JENV file removed.
const JES = "jes"

// LogErrorStack is a developer feature flag to have the LoggedErrorStack
// function in the utils package write out the error stack as defined by the
// errors package to the logger.  The ability to log the error stack is very
// useful in those error cases where you really don't expect there to be a
// failure.  This means that the developers with this flag set will see the
// stack trace in the log output, but normal deployments never will.
const LogErrorStack = "log-error-stack"

// LegacyUpstart is used to indicate that the version-based init system
// discovery code (service.VersionInitSystem) should return upstart
// instead of systemd for vivid and newer.
const LegacyUpstart = "legacy-upstart"

// AddressAllocation is used to indicate that LXC and KVM containers
// on providers that support that (currently only MAAS and EC2) will
// use statically allocated IP addresses.
const AddressAllocation = "address-allocation"

// PostNetCLIMVP is used to indicate that additional networking
// commands will be available in addition to the networking MVP ones
// (space list|create, subnet list|add).
const PostNetCLIMVP = "post-net-cli-mvp"

// dbLog indicates that Juju's logs go to MongoDB. It is not exported
// because it should be checked for using IsDbLogEnabled.
const dbLog = "db-log"

// IsDbLogEnabled returns true if logging to MongoDB should be enabled
// based on the dbLog or JES feature flags.
func IsDbLogEnabled() bool {
	return featureflag.Enabled(dbLog) || featureflag.Enabled(JES)
}

// DisableRsyslog will stop the writing of the rsyslog accumulation and
// forwarding configuration files by stopping the rsyslog workers.
const DisableRsyslog = "disable-rsyslog"

// CloudSigma enables the CloudSigma provider.
const CloudSigma = "cloudsigma"

// VSphereProvider enables the generic vmware provider.
const VSphereProvider = "vsphere-provider"
