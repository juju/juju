// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"

	"github.com/juju/schema"

	"github.com/juju/juju/internal/configschema"
)

var configChecker = schema.FieldMap(schema.Fields{
	AgentRateLimitMax:                  schema.ForceInt(),
	AgentRateLimitRate:                 schema.TimeDurationString(),
	AuditingEnabled:                    schema.Bool(),
	AuditLogCaptureArgs:                schema.Bool(),
	AuditLogMaxSize:                    schema.String(),
	AuditLogMaxBackups:                 schema.ForceInt(),
	AuditLogExcludeMethods:             schema.String(),
	APIPort:                            schema.ForceInt(),
	ControllerName:                     schema.NonEmptyString(ControllerName),
	StatePort:                          schema.ForceInt(),
	LoginTokenRefreshURL:               schema.String(),
	IdentityURL:                        schema.String(),
	IdentityPublicKey:                  schema.String(),
	SetNUMAControlPolicyKey:            schema.Bool(),
	AutocertURLKey:                     schema.String(),
	AutocertDNSNameKey:                 schema.String(),
	AllowModelAccessKey:                schema.Bool(),
	JujuDBSnapChannel:                  schema.String(),
	MaxDebugLogDuration:                schema.TimeDurationString(),
	MaxTxnLogSize:                      schema.String(),
	MaxPruneTxnBatchSize:               schema.ForceInt(),
	MaxPruneTxnPasses:                  schema.ForceInt(),
	AgentLogfileMaxBackups:             schema.ForceInt(),
	AgentLogfileMaxSize:                schema.String(),
	ModelLogfileMaxBackups:             schema.ForceInt(),
	ModelLogfileMaxSize:                schema.String(),
	PruneTxnQueryCount:                 schema.ForceInt(),
	PruneTxnSleepTime:                  schema.TimeDurationString(),
	PublicDNSAddress:                   schema.String(),
	JujuManagementSpace:                schema.String(),
	CAASOperatorImagePath:              schema.String(),
	CAASImageRepo:                      schema.String(),
	Features:                           schema.String(),
	MaxCharmStateSize:                  schema.ForceInt(),
	MaxAgentStateSize:                  schema.ForceInt(),
	MigrationMinionWaitMax:             schema.TimeDurationString(),
	ApplicationResourceDownloadLimit:   schema.ForceInt(),
	ControllerResourceDownloadLimit:    schema.ForceInt(),
	QueryTracingEnabled:                schema.Bool(),
	QueryTracingThreshold:              schema.TimeDurationString(),
	OpenTelemetryEnabled:               schema.Bool(),
	OpenTelemetryEndpoint:              schema.String(),
	OpenTelemetryInsecure:              schema.Bool(),
	OpenTelemetryStackTraces:           schema.Bool(),
	OpenTelemetrySampleRatio:           schema.String(),
	OpenTelemetryTailSamplingThreshold: schema.TimeDurationString(),
	ObjectStoreType:                    schema.String(),
	ObjectStoreS3Endpoint:              schema.String(),
	ObjectStoreS3StaticKey:             schema.String(),
	ObjectStoreS3StaticSecret:          schema.String(),
	ObjectStoreS3StaticSession:         schema.String(),
	SystemSSHKeys:                      schema.String(),
	JujudControllerSnapSource:          schema.String(),
	SSHServerPort:                      schema.ForceInt(),
	SSHMaxConcurrentConnections:        schema.ForceInt(),
}, schema.Defaults{
	AgentRateLimitMax:                  schema.Omit,
	AgentRateLimitRate:                 schema.Omit,
	APIPort:                            DefaultAPIPort,
	ControllerName:                     schema.Omit,
	AuditingEnabled:                    DefaultAuditingEnabled,
	AuditLogCaptureArgs:                DefaultAuditLogCaptureArgs,
	AuditLogMaxSize:                    fmt.Sprintf("%vM", DefaultAuditLogMaxSizeMB),
	AuditLogMaxBackups:                 DefaultAuditLogMaxBackups,
	AuditLogExcludeMethods:             DefaultAuditLogExcludeMethods,
	StatePort:                          DefaultStatePort,
	LoginTokenRefreshURL:               schema.Omit,
	IdentityURL:                        schema.Omit,
	IdentityPublicKey:                  schema.Omit,
	SetNUMAControlPolicyKey:            DefaultNUMAControlPolicy,
	AutocertURLKey:                     schema.Omit,
	AutocertDNSNameKey:                 schema.Omit,
	AllowModelAccessKey:                schema.Omit,
	JujuDBSnapChannel:                  DefaultJujuDBSnapChannel,
	MaxDebugLogDuration:                DefaultMaxDebugLogDuration,
	MaxTxnLogSize:                      fmt.Sprintf("%vM", DefaultMaxTxnLogCollectionMB),
	MaxPruneTxnBatchSize:               DefaultMaxPruneTxnBatchSize,
	MaxPruneTxnPasses:                  DefaultMaxPruneTxnPasses,
	AgentLogfileMaxBackups:             DefaultAgentLogfileMaxBackups,
	AgentLogfileMaxSize:                fmt.Sprintf("%vM", DefaultAgentLogfileMaxSize),
	ModelLogfileMaxBackups:             DefaultModelLogfileMaxBackups,
	ModelLogfileMaxSize:                fmt.Sprintf("%vM", DefaultModelLogfileMaxSize),
	PruneTxnQueryCount:                 DefaultPruneTxnQueryCount,
	PruneTxnSleepTime:                  DefaultPruneTxnSleepTime,
	PublicDNSAddress:                   schema.Omit,
	JujuManagementSpace:                schema.Omit,
	CAASOperatorImagePath:              schema.Omit,
	CAASImageRepo:                      schema.Omit,
	Features:                           schema.Omit,
	MaxCharmStateSize:                  DefaultMaxCharmStateSize,
	MaxAgentStateSize:                  DefaultMaxAgentStateSize,
	MigrationMinionWaitMax:             DefaultMigrationMinionWaitMax,
	ApplicationResourceDownloadLimit:   schema.Omit,
	ControllerResourceDownloadLimit:    schema.Omit,
	QueryTracingEnabled:                DefaultQueryTracingEnabled,
	QueryTracingThreshold:              DefaultQueryTracingThreshold,
	OpenTelemetryEnabled:               DefaultOpenTelemetryEnabled,
	OpenTelemetryEndpoint:              schema.Omit,
	OpenTelemetryInsecure:              DefaultOpenTelemetryInsecure,
	OpenTelemetryStackTraces:           DefaultOpenTelemetryStackTraces,
	OpenTelemetrySampleRatio:           fmt.Sprintf("%.02f", DefaultOpenTelemetrySampleRatio),
	OpenTelemetryTailSamplingThreshold: DefaultOpenTelemetryTailSamplingThreshold,
	ObjectStoreType:                    DefaultObjectStoreType,
	ObjectStoreS3Endpoint:              schema.Omit,
	ObjectStoreS3StaticKey:             schema.Omit,
	ObjectStoreS3StaticSecret:          schema.Omit,
	ObjectStoreS3StaticSession:         schema.Omit,
	SystemSSHKeys:                      schema.Omit,
	JujudControllerSnapSource:          DefaultJujudControllerSnapSource,
	SSHServerPort:                      DefaultSSHServerPort,
	SSHMaxConcurrentConnections:        DefaultSSHMaxConcurrentConnections,
})

// ConfigSchema holds information on all the fields defined by
// the config package.
var ConfigSchema = configschema.Fields{
	ApplicationResourceDownloadLimit: {
		Description: "The maximum number of concurrent resources downloads per application",
		Type:        configschema.Tint,
	},
	ControllerResourceDownloadLimit: {
		Description: "The maximum number of concurrent resources downloads across all the applications on the controller",
		Type:        configschema.Tint,
	},
	AgentRateLimitMax: {
		Description: "The maximum size of the token bucket used to ratelimit agent connections",
		Type:        configschema.Tint,
	},
	AgentRateLimitRate: {
		Description: "The time taken to add a new token to the ratelimit bucket",
		Type:        configschema.Tstring,
	},
	AuditingEnabled: {
		Description: "Determines if the controller records auditing information",
		Type:        configschema.Tbool,
	},
	AuditLogCaptureArgs: {
		Description: `Determines if the audit log contains the arguments passed to API methods`,
		Type:        configschema.Tbool,
	},
	AuditLogMaxSize: {
		Description: "The maximum size for the current controller audit log file",
		Type:        configschema.Tstring,
	},
	AuditLogMaxBackups: {
		Type:        configschema.Tint,
		Description: "The number of old audit log files to keep (compressed)",
	},
	AuditLogExcludeMethods: {
		Type:        configschema.Tstring,
		Description: "A comma-delimited list of Facade.Method names that aren't interesting for audit logging purposes.",
	},
	APIPort: {
		Type:        configschema.Tint,
		Description: "The port used for api connections",
	},
	ControllerName: {
		Type:        configschema.Tstring,
		Description: `The canonical name of the controller`,
	},
	StatePort: {
		Type:        configschema.Tint,
		Description: `The port used for mongo connections`,
	},
	LoginTokenRefreshURL: {
		Type:        configschema.Tstring,
		Description: `The url of the jwt well known endpoint`,
	},
	IdentityURL: {
		Type:        configschema.Tstring,
		Description: `The url of the identity manager`,
	},
	IdentityPublicKey: {
		Type:        configschema.Tstring,
		Description: `The public key of the identity manager`,
	},
	SetNUMAControlPolicyKey: {
		Type:        configschema.Tbool,
		Description: `Determines if the NUMA control policy is set`,
	},
	AutocertURLKey: {
		Type:        configschema.Tstring,
		Description: `The URL used to obtain official TLS certificates when a client connects to the API`,
	},
	AutocertDNSNameKey: {
		Type:        configschema.Tstring,
		Description: `The DNS name of the controller`,
	},
	AllowModelAccessKey: {
		Type: configschema.Tbool,
		Description: `Determines if the controller allows users to
connect to models they have been authorized for even when
they don't have any access rights to the controller itself`,
	},
	JujuDBSnapChannel: {
		Type:        configschema.Tstring,
		Description: `Sets channel for installing mongo snaps when bootstrapping on focal or later`,
	},
	MaxDebugLogDuration: {
		Type:        configschema.Tstring,
		Description: `The maximum duration that a debug-log session is allowed to run`,
	},
	MaxTxnLogSize: {
		Type:        configschema.Tstring,
		Description: `The maximum size the of capped txn log collection`,
	},
	MaxPruneTxnBatchSize: {
		Type:        configschema.Tint,
		Description: `(deprecated) The maximum number of transactions evaluated in one go when pruning`,
	},
	MaxPruneTxnPasses: {
		Type:        configschema.Tint,
		Description: `(deprecated) The maximum number of batches processed when pruning`,
	},
	AgentLogfileMaxBackups: {
		Type:        configschema.Tint,
		Description: "The number of old agent log files to keep (compressed)",
	},
	AgentLogfileMaxSize: {
		Type:        configschema.Tstring,
		Description: `The maximum size of the agent log file`,
	},
	ModelLogfileMaxBackups: {
		Type:        configschema.Tint,
		Description: "The number of old model log files to keep (compressed)",
	},
	ModelLogfileMaxSize: {
		Type:        configschema.Tstring,
		Description: `The maximum size of the log file written out by the controller on behalf of workers running for a model`,
	},
	PruneTxnQueryCount: {
		Type:        configschema.Tint,
		Description: `The number of transactions to read in a single query`,
	},
	PruneTxnSleepTime: {
		Type:        configschema.Tstring,
		Description: `The amount of time to sleep between processing each batch query`,
	},
	PublicDNSAddress: {
		Type:        configschema.Tstring,
		Description: `Public DNS address (with port) of the controller.`,
	},
	JujuManagementSpace: {
		Type:        configschema.Tstring,
		Description: `The network space that agents should use to communicate with controllers`,
	},
	CAASOperatorImagePath: {
		Type: configschema.Tstring,
		Description: `(deprecated) The url of the docker image used for the application operator.
Use "caas-image-repo" instead.`,
	},
	CAASImageRepo: {
		Type:        configschema.Tstring,
		Description: `The docker repo to use for the jujud operator and mongo images`,
	},
	Features: {
		Type:        configschema.Tstring,
		Description: `A comma-delimited list of runtime changeable features to be updated`,
	},
	MaxCharmStateSize: {
		Type:        configschema.Tint,
		Description: `The maximum size (in bytes) of charm-specific state that units can store to the controller`,
	},
	MaxAgentStateSize: {
		Type:        configschema.Tint,
		Description: `The maximum size (in bytes) of internal state data that agents can store to the controller`,
	},
	MigrationMinionWaitMax: {
		Type:        configschema.Tstring,
		Description: `The maximum during model migrations that the migration worker will wait for agents to report on phases of the migration`,
	},
	QueryTracingEnabled: {
		Type:        configschema.Tbool,
		Description: `Enable query tracing for the dqlite driver`,
	},
	QueryTracingThreshold: {
		Type: configschema.Tstring,
		Description: `The minimum duration of a query for it to be traced. The lower the
threshold, the more queries will be output. A value of 0 means all queries
will be output if tracing is enabled.`,
	},
	OpenTelemetryEnabled: {
		Type:        configschema.Tbool,
		Description: `Enable open telemetry tracing`,
	},
	OpenTelemetryEndpoint: {
		Type:        configschema.Tstring,
		Description: `Endpoint open telemetry tracing`,
	},
	OpenTelemetryInsecure: {
		Type:        configschema.Tbool,
		Description: `Allows insecure endpoint for open telemetry tracing`,
	},
	OpenTelemetryStackTraces: {
		Type:        configschema.Tbool,
		Description: `Allows stack traces open telemetry tracing per span`,
	},
	OpenTelemetrySampleRatio: {
		Type:        configschema.Tstring,
		Description: `Allows defining a sample ratio open telemetry tracing`,
	},
	OpenTelemetryTailSamplingThreshold: {
		Type:        configschema.Tstring,
		Description: "Allows defining a tail sampling threshold open telemetry tracing",
	},
	ObjectStoreType: {
		Type:        configschema.Tstring,
		Description: `The type of object store backend to use for storing blobs`,
	},
	ObjectStoreS3Endpoint: {
		Type:        configschema.Tstring,
		Description: `The s3 endpoint for the object store backend`,
	},
	ObjectStoreS3StaticKey: {
		Type:        configschema.Tstring,
		Description: `The s3 static key for the object store backend`,
	},
	ObjectStoreS3StaticSecret: {
		Type:        configschema.Tstring,
		Description: `The s3 static secret for the object store backend`,
	},
	ObjectStoreS3StaticSession: {
		Type:        configschema.Tstring,
		Description: `The s3 static session for the object store backend`,
	},
	SystemSSHKeys: {
		Type:        configschema.Tstring,
		Description: `Defines the system ssh keys`,
	},
	JujudControllerSnapSource: {
		Type:        configschema.Tstring,
		Description: `The source for the jujud-controller snap.`,
	},
	SSHServerPort: {
		Type:        configschema.Tint,
		Description: `The port used for ssh connections to the controller`,
	},
	SSHMaxConcurrentConnections: {
		Type:        configschema.Tint,
		Description: `The maximum number of concurrent ssh connections to the controller`,
	},
}
