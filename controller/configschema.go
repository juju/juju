// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"

	"github.com/juju/romulus"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"
)

var configFields = schema.Fields{
	AgentRateLimitMax:                schema.ForceInt(),
	AgentRateLimitRate:               schema.TimeDuration(),
	AuditingEnabled:                  schema.Bool(),
	AuditLogCaptureArgs:              schema.Bool(),
	AuditLogMaxSize:                  schema.String(),
	AuditLogMaxBackups:               schema.ForceInt(),
	AuditLogExcludeMethods:           schema.String(),
	APIPort:                          schema.ForceInt(),
	APIPortOpenDelay:                 schema.TimeDuration(),
	ControllerAPIPort:                schema.ForceInt(),
	ControllerName:                   schema.String(),
	StatePort:                        schema.ForceInt(),
	LoginTokenRefreshURL:             schema.String(),
	IdentityURL:                      schema.String(),
	IdentityPublicKey:                schema.String(),
	SetNUMAControlPolicyKey:          schema.Bool(),
	AutocertURLKey:                   schema.String(),
	AutocertDNSNameKey:               schema.String(),
	AllowModelAccessKey:              schema.Bool(),
	MongoMemoryProfile:               schema.String(),
	JujuDBSnapChannel:                schema.String(),
	MaxDebugLogDuration:              schema.TimeDuration(),
	MaxTxnLogSize:                    schema.String(),
	MaxPruneTxnBatchSize:             schema.ForceInt(),
	MaxPruneTxnPasses:                schema.ForceInt(),
	AgentLogfileMaxBackups:           schema.ForceInt(),
	AgentLogfileMaxSize:              schema.String(),
	ModelLogfileMaxBackups:           schema.ForceInt(),
	ModelLogfileMaxSize:              schema.String(),
	ModelLogsSize:                    schema.String(),
	PruneTxnQueryCount:               schema.ForceInt(),
	PruneTxnSleepTime:                schema.TimeDuration(),
	PublicDNSAddress:                 schema.String(),
	JujuHASpace:                      schema.String(),
	JujuManagementSpace:              schema.String(),
	CAASOperatorImagePath:            schema.String(),
	CAASImageRepo:                    schema.String(),
	Features:                         schema.String(),
	MeteringURL:                      schema.String(),
	MaxCharmStateSize:                schema.ForceInt(),
	MaxAgentStateSize:                schema.ForceInt(),
	MigrationMinionWaitMax:           schema.TimeDuration(),
	ApplicationResourceDownloadLimit: schema.ForceInt(),
	ControllerResourceDownloadLimit:  schema.ForceInt(),
	QueryTracingEnabled:              schema.Bool(),
	QueryTracingThreshold:            schema.TimeDuration(),
	OpenTelemetryEnabled:             schema.Bool(),
	OpenTelemetryEndpoint:            schema.String(),
	OpenTelemetryInsecure:            schema.Bool(),
	OpenTelemetryStackTraces:         schema.Bool(),
}

var configChecker = schema.FieldMap(
	configFields,
	schema.Defaults{
		AgentRateLimitMax:                schema.Omit,
		AgentRateLimitRate:               schema.Omit,
		APIPort:                          DefaultAPIPort,
		APIPortOpenDelay:                 DefaultAPIPortOpenDelay,
		ControllerAPIPort:                schema.Omit,
		ControllerName:                   schema.Omit,
		AuditingEnabled:                  DefaultAuditingEnabled,
		AuditLogCaptureArgs:              DefaultAuditLogCaptureArgs,
		AuditLogMaxSize:                  fmt.Sprintf("%vM", DefaultAuditLogMaxSizeMB),
		AuditLogMaxBackups:               DefaultAuditLogMaxBackups,
		AuditLogExcludeMethods:           DefaultAuditLogExcludeMethods,
		StatePort:                        DefaultStatePort,
		LoginTokenRefreshURL:             schema.Omit,
		IdentityURL:                      schema.Omit,
		IdentityPublicKey:                schema.Omit,
		SetNUMAControlPolicyKey:          DefaultNUMAControlPolicy,
		AutocertURLKey:                   schema.Omit,
		AutocertDNSNameKey:               schema.Omit,
		AllowModelAccessKey:              schema.Omit,
		MongoMemoryProfile:               DefaultMongoMemoryProfile,
		JujuDBSnapChannel:                DefaultJujuDBSnapChannel,
		MaxDebugLogDuration:              DefaultMaxDebugLogDuration,
		MaxTxnLogSize:                    fmt.Sprintf("%vM", DefaultMaxTxnLogCollectionMB),
		MaxPruneTxnBatchSize:             DefaultMaxPruneTxnBatchSize,
		MaxPruneTxnPasses:                DefaultMaxPruneTxnPasses,
		AgentLogfileMaxBackups:           DefaultAgentLogfileMaxBackups,
		AgentLogfileMaxSize:              fmt.Sprintf("%vM", DefaultAgentLogfileMaxSize),
		ModelLogfileMaxBackups:           DefaultModelLogfileMaxBackups,
		ModelLogfileMaxSize:              fmt.Sprintf("%vM", DefaultModelLogfileMaxSize),
		ModelLogsSize:                    fmt.Sprintf("%vM", DefaultModelLogsSizeMB),
		PruneTxnQueryCount:               DefaultPruneTxnQueryCount,
		PruneTxnSleepTime:                DefaultPruneTxnSleepTime,
		PublicDNSAddress:                 schema.Omit,
		JujuHASpace:                      schema.Omit,
		JujuManagementSpace:              schema.Omit,
		CAASOperatorImagePath:            schema.Omit,
		CAASImageRepo:                    schema.Omit,
		Features:                         schema.Omit,
		MeteringURL:                      romulus.DefaultAPIRoot,
		MaxCharmStateSize:                DefaultMaxCharmStateSize,
		MaxAgentStateSize:                DefaultMaxAgentStateSize,
		MigrationMinionWaitMax:           DefaultMigrationMinionWaitMax,
		ApplicationResourceDownloadLimit: schema.Omit,
		ControllerResourceDownloadLimit:  schema.Omit,
		QueryTracingEnabled:              DefaultQueryTracingEnabled,
		QueryTracingThreshold:            DefaultQueryTracingThreshold,
		OpenTelemetryEnabled:             DefaultOpenTelemetryEnabled,
		OpenTelemetryEndpoint:            schema.Omit,
		OpenTelemetryInsecure:            DefaultOpenTelemetryInsecure,
		OpenTelemetryStackTraces:         DefaultOpenTelemetryStackTraces,
	},
)

// ConfigSchema holds information on all the fields defined by
// the config package.
var ConfigSchema = environschema.Fields{
	ApplicationResourceDownloadLimit: {
		Description: "The maximum number of concurrent resources downloads per application",
		Type:        environschema.Tint,
	},
	ControllerResourceDownloadLimit: {
		Description: "The maximum number of concurrent resources downloads across all the applications on the controller",
		Type:        environschema.Tint,
	},
	AgentRateLimitMax: {
		Description: "The maximum size of the token bucket used to ratelimit agent connections",
		Type:        environschema.Tint,
	},
	AgentRateLimitRate: {
		Description: "The time taken to add a new token to the ratelimit bucket",
		Type:        environschema.Tstring,
	},
	AuditingEnabled: {
		Description: "Determines if the controller records auditing information",
		Type:        environschema.Tbool,
	},
	AuditLogCaptureArgs: {
		Description: `Determines if the audit log contains the arguments passed to API methods`,
		Type:        environschema.Tbool,
	},
	AuditLogMaxSize: {
		Description: "The maximum size for the current controller audit log file",
		Type:        environschema.Tstring,
	},
	AuditLogMaxBackups: {
		Type:        environschema.Tint,
		Description: "The number of old audit log files to keep (compressed)",
	},
	AuditLogExcludeMethods: {
		Type:        environschema.Tstring,
		Description: "A comma-delimited list of Facade.Method names that aren't interesting for audit logging purposes.",
	},
	APIPort: {
		Type:        environschema.Tint,
		Description: "The port used for api connections",
	},
	APIPortOpenDelay: {
		Type: environschema.Tstring,
		Description: `The duration that the controller will wait 
between when the controller has been deemed to be ready to open 
the api-port and when the api-port is actually opened 
(only used when a controller-api-port value is set).`,
	},
	ControllerAPIPort: {
		Type: environschema.Tint,
		Description: `An optional port that may be set for controllers
that have a very heavy load. If this port is set, this port is used by
the controllers to talk to each other - used for the local API connection
as well as the pubsub forwarders, and the raft workers. If this value is
set, the api-port isn't opened until the controllers have started properly.`,
	},
	StatePort: {
		Type:        environschema.Tint,
		Description: `The port used for mongo connections`,
	},
	LoginTokenRefreshURL: {
		Type:        environschema.Tstring,
		Description: `The url of the jwt well known endpoint`,
	},
	IdentityURL: {
		Type:        environschema.Tstring,
		Description: `The url of the identity manager`,
	},
	IdentityPublicKey: {
		Type:        environschema.Tstring,
		Description: `The public key of the identity manager`,
	},
	SetNUMAControlPolicyKey: {
		Type:        environschema.Tbool,
		Description: `Determines if the NUMA control policy is set`,
	},
	AutocertURLKey: {
		Type:        environschema.Tstring,
		Description: `The URL used to obtain official TLS certificates when a client connects to the API`,
	},
	AutocertDNSNameKey: {
		Type:        environschema.Tstring,
		Description: `The DNS name of the controller`,
	},
	AllowModelAccessKey: {
		Type: environschema.Tbool,
		Description: `Determines if the controller allows users to 
connect to models they have been authorized for even when 
they don't have any access rights to the controller itself`,
	},
	MongoMemoryProfile: {
		Type:        environschema.Tstring,
		Description: `Sets mongo memory profile`,
	},
	JujuDBSnapChannel: {
		Type:        environschema.Tstring,
		Description: `Sets channel for installing mongo snaps when bootstrapping on focal or later`,
	},
	MaxDebugLogDuration: {
		Type:        environschema.Tstring,
		Description: `The maximum duration that a debug-log session is allowed to run`,
	},
	MaxTxnLogSize: {
		Type:        environschema.Tstring,
		Description: `The maximum size the of capped txn log collection`,
	},
	MaxPruneTxnBatchSize: {
		Type:        environschema.Tint,
		Description: `(deprecated) The maximum number of transactions evaluated in one go when pruning`,
	},
	MaxPruneTxnPasses: {
		Type:        environschema.Tint,
		Description: `(deprecated) The maximum number of batches processed when pruning`,
	},
	AgentLogfileMaxBackups: {
		Type:        environschema.Tint,
		Description: "The number of old agent log files to keep (compressed)",
	},
	AgentLogfileMaxSize: {
		Type:        environschema.Tstring,
		Description: `The maximum size of the agent log file`,
	},
	ModelLogfileMaxBackups: {
		Type:        environschema.Tint,
		Description: "The number of old model log files to keep (compressed)",
	},
	ModelLogfileMaxSize: {
		Type:        environschema.Tstring,
		Description: `The maximum size of the log file written out by the controller on behalf of workers running for a model`,
	},
	ModelLogsSize: {
		Type:        environschema.Tstring,
		Description: `The size of the capped collections used to hold the logs for the models`,
	},
	PruneTxnQueryCount: {
		Type:        environschema.Tint,
		Description: `The number of transactions to read in a single query`,
	},
	PruneTxnSleepTime: {
		Type:        environschema.Tstring,
		Description: `The amount of time to sleep between processing each batch query`,
	},
	PublicDNSAddress: {
		Type:        environschema.Tstring,
		Description: `Public DNS address (with port) of the controller.`,
	},
	JujuHASpace: {
		Type:        environschema.Tstring,
		Description: `The network space within which the MongoDB replica-set should communicate`,
	},
	JujuManagementSpace: {
		Type:        environschema.Tstring,
		Description: `The network space that agents should use to communicate with controllers`,
	},
	CAASOperatorImagePath: {
		Type: environschema.Tstring,
		Description: `(deprecated) The url of the docker image used for the application operator.
Use "caas-image-repo" instead.`,
	},
	CAASImageRepo: {
		Type:        environschema.Tstring,
		Description: `The docker repo to use for the jujud operator and mongo images`,
	},
	Features: {
		Type:        environschema.Tstring,
		Description: `A comma-delimited list of runtime changeable features to be updated`,
	},
	MeteringURL: {
		Type:        environschema.Tstring,
		Description: `The url for metrics`,
	},
	MaxCharmStateSize: {
		Type:        environschema.Tint,
		Description: `The maximum size (in bytes) of charm-specific state that units can store to the controller`,
	},
	MaxAgentStateSize: {
		Type:        environschema.Tint,
		Description: `The maximum size (in bytes) of internal state data that agents can store to the controller`,
	},
	MigrationMinionWaitMax: {
		Type:        environschema.Tstring,
		Description: `The maximum during model migrations that the migration worker will wait for agents to report on phases of the migration`,
	},
	QueryTracingEnabled: {
		Type:        environschema.Tbool,
		Description: `Enable query tracing for the dqlite driver`,
	},
	QueryTracingThreshold: {
		Type: environschema.Tstring,
		Description: `The minimum duration of a query for it to be traced. The lower the 
threshold, the more queries will be output. A value of 0 means all queries 
will be output if tracing is enabled.`,
	},
	OpenTelemetryEnabled: {
		Type:        environschema.Tbool,
		Description: `Enable open telemetry tracing`,
	},
	OpenTelemetryEndpoint: {
		Type:        environschema.Tstring,
		Description: `Endpoint open telemetry tracing`,
	},
	OpenTelemetryInsecure: {
		Type:        environschema.Tbool,
		Description: `Allows insecure endpoint for open telemetry tracing`,
	},
	OpenTelemetryStackTraces: {
		Type:        environschema.Tbool,
		Description: `Allows stack traces open telemetry tracing per span`,
	},
}
