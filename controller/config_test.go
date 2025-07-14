// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/internal/docker/registry/mocks"
	"github.com/juju/juju/internal/testing"
)

type ConfigSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func TestConfigSuite(t *stdtesting.T) {
	tc.Run(t, &ConfigSuite{})
}

func (s *ConfigSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	// Make sure that the defaults are used, which
	// is <root>=WARNING
	loggo.DefaultContext().ResetLoggerLevels()
}

var validateTests = []struct {
	about       string
	config      controller.Config
	expectError string
}{{
	about:       "missing CA cert",
	expectError: `missing CA certificate`,
}, {
	about: "bad CA cert",
	config: controller.Config{
		controller.CACertKey: "xxx",
	},
	expectError: `bad CA certificate in configuration: no certificates in pem bundle`,
}, {
	about: "bad controller UUID",
	config: controller.Config{
		controller.ControllerUUIDKey: "xxx",
		controller.CACertKey:         testing.CACert,
	},
	expectError: `controller-uuid: expected UUID, got string\("xxx"\)`,
}}

func (s *ConfigSuite) TestValidate(c *tc.C) {
	// Normally Validate is only called as part of the NewConfig call, which
	// also does schema coercing. The NewConfig method takes the controller uuid
	// and cacert as separate args, so to get invalid ones, we skip that part.
	for i, test := range validateTests {
		c.Logf("test %d: %v", i, test.about)
		err := test.config.Validate()
		if test.expectError != "" {
			c.Check(err, tc.ErrorMatches, test.expectError)
		} else {
			c.Check(err, tc.ErrorIsNil)
		}
	}
}

var newConfigTests = []struct {
	about       string
	config      controller.Config
	expectError string
}{{
	about: "HTTPS identity URL OK",
	config: controller.Config{
		controller.IdentityURL: "https://0.1.2.3/foo",
	},
}, {
	about: "HTTP identity URL requires public key",
	config: controller.Config{
		controller.IdentityURL: "http://0.1.2.3/foo",
	},
	expectError: `URL needs to be https when identity-public-key not provided`,
}, {
	about: "HTTP identity URL OK if public key is provided",
	config: controller.Config{
		controller.IdentityPublicKey: `o/yOqSNWncMo1GURWuez/dGR30TscmmuIxgjztpoHEY=`,
		controller.IdentityURL:       "http://0.1.2.3/foo",
	},
}, {
	about: "invalid identity public key",
	config: controller.Config{
		controller.IdentityPublicKey: `xxxx`,
	},
	expectError: `invalid identity public key: wrong length for key, got 3 want 32`,
}, {
	about: "invalid management space name - whitespace",
	config: controller.Config{
		controller.JujuManagementSpace: " ",
	},
	expectError: `juju mgmt space name " " not valid`,
}, {
	about: "invalid management space name - caps",
	config: controller.Config{
		controller.JujuManagementSpace: "CAPS",
	},
	expectError: `juju mgmt space name "CAPS" not valid`,
}, {
	about: "invalid management space name - carriage return",
	config: controller.Config{
		controller.JujuManagementSpace: "\n",
	},
	expectError: `juju mgmt space name "\\n" not valid`,
}, {
	about: "invalid audit log max size",
	config: controller.Config{
		controller.AuditLogMaxSize: "abcd",
	},
	expectError: `invalid audit log max size in configuration: expected a non-negative number, got "abcd"`,
}, {
	about: "zero audit log max size",
	config: controller.Config{
		controller.AuditingEnabled: true,
		controller.AuditLogMaxSize: "0M",
	},
	expectError: `invalid audit log max size: can't be 0 if auditing is enabled`,
}, {
	about: "invalid audit log max backups",
	config: controller.Config{
		controller.AuditLogMaxBackups: -10,
	},
	expectError: `invalid audit log max backups: should be a number of files \(or 0 to keep all\), got -10`,
}, {
	about: "invalid audit log exclude",
	config: controller.Config{
		controller.AuditLogExcludeMethods: "Dap.Kings,ReadOnlyMethods,Sharon Jones",
	},
	expectError: `invalid audit log exclude methods: should be a list of "Facade.Method" names \(or "ReadOnlyMethods"\), got "Sharon Jones" at position 3`,
}, {
	about: "txn-prune-sleep-time not a duration",
	config: controller.Config{
		controller.PruneTxnSleepTime: "15",
	},
	expectError: `prune-txn-sleep-time: conversion to duration: time: missing unit in duration "15"`,
}, {
	about: "max-debug-log-duration not valid",
	config: controller.Config{
		controller.MaxDebugLogDuration: time.Duration(0),
	},
	expectError: `max-debug-log-duration cannot be zero`,
}, {
	about: "agent-logfile-max-backups not valid",
	config: controller.Config{
		controller.AgentLogfileMaxBackups: -1,
	},
	expectError: `negative agent-logfile-max-backups not valid`,
}, {
	about: "agent-logfile-max-size not valid",
	config: controller.Config{
		controller.AgentLogfileMaxSize: "0",
	},
	expectError: `agent-logfile-max-size less than 1 MB not valid`,
}, {
	about: "model-logfile-max-backups not valid",
	config: controller.Config{
		controller.ModelLogfileMaxBackups: -1,
	},
	expectError: `negative model-logfile-max-backups not valid`,
}, {
	about: "model-logfile-max-size not valid",
	config: controller.Config{
		controller.ModelLogfileMaxSize: "0",
	},
	expectError: `model-logfile-max-size less than 1 MB not valid`,
}, {
	about: "agent-ratelimit-max non-int",
	config: controller.Config{
		controller.AgentRateLimitMax: "ten",
	},
	expectError: `agent-ratelimit-max: expected number, got string\("ten"\)`,
}, {
	about: "agent-ratelimit-max negative",
	config: controller.Config{
		controller.AgentRateLimitMax: "-5",
	},
	expectError: `negative agent-ratelimit-max \(-5\) not valid`,
}, {
	about: "agent-ratelimit-rate missing unit",
	config: controller.Config{
		controller.AgentRateLimitRate: "150",
	},
	expectError: `agent-ratelimit-rate: conversion to duration: time: missing unit in duration "?150"?`,
}, {
	about: "agent-ratelimit-rate bad type, int",
	config: controller.Config{
		controller.AgentRateLimitRate: 150,
	},
	expectError: `agent-ratelimit-rate: expected string or time.Duration, got int\(150\)`,
}, {
	about: "agent-ratelimit-rate zero",
	config: controller.Config{
		controller.AgentRateLimitRate: "0s",
	},
	expectError: `agent-ratelimit-rate cannot be zero`,
}, {
	about: "agent-ratelimit-rate negative",
	config: controller.Config{
		controller.AgentRateLimitRate: "-5s",
	},
	expectError: `agent-ratelimit-rate cannot be negative`,
}, {
	about: "agent-ratelimit-rate too large",
	config: controller.Config{
		controller.AgentRateLimitRate: "4h",
	},
	expectError: `agent-ratelimit-rate must be between 0..1m`,
}, {
	about: "max-charm-state-size non-int",
	config: controller.Config{
		controller.MaxCharmStateSize: "ten",
	},
	expectError: `max-charm-state-size: expected number, got string\("ten"\)`,
}, {
	about: "max-charm-state-size cannot be negative",
	config: controller.Config{
		controller.MaxCharmStateSize: "-42",
	},
	expectError: `invalid max charm state size: should be a number of bytes \(or 0 to disable limit\), got -42`,
}, {
	about: "max-agent-state-size non-int",
	config: controller.Config{
		controller.MaxAgentStateSize: "ten",
	},
	expectError: `max-agent-state-size: expected number, got string\("ten"\)`,
}, {
	about: "max-agent-state-size cannot be negative",
	config: controller.Config{
		controller.MaxAgentStateSize: "-42",
	},
	expectError: `invalid max agent state size: should be a number of bytes \(or 0 to disable limit\), got -42`,
}, {
	about: "combined charm/agent state cannot exceed mongo's 16M limit/doc",
	config: controller.Config{
		controller.MaxCharmStateSize: "14000000",
		controller.MaxAgentStateSize: "3000000",
	},
	expectError: `invalid max charm/agent state sizes: combined value should not exceed mongo's 16M per-document limit, got 17000000`,
}, {
	about: "public-dns-address: expect string, got number",
	config: controller.Config{
		controller.PublicDNSAddress: 42,
	},
	expectError: `public-dns-address: expected string, got int\(42\)`,
}, {
	about: "migration-agent-wait-time not a duration",
	config: controller.Config{
		controller.MigrationMinionWaitMax: "15",
	},
	expectError: `migration-agent-wait-time: conversion to duration: time: missing unit in duration "15"`,
}, {
	about: "application-resource-download-limit cannot be negative",
	config: controller.Config{
		controller.ApplicationResourceDownloadLimit: "-42",
	},
	expectError: `negative application-resource-download-limit \(-42\) not valid, use 0 to disable the limit`,
}, {
	about: "controller-resource-download-limit cannot be negative",
	config: controller.Config{
		controller.ControllerResourceDownloadLimit: "-42",
	},
	expectError: `negative controller-resource-download-limit \(-42\) not valid, use 0 to disable the limit`,
}, {
	about: "login token refresh url",
	config: controller.Config{
		controller.LoginTokenRefreshURL: `https://xxxx`,
	},
}, {
	about: "invalid login token refresh url",
	config: controller.Config{
		controller.LoginTokenRefreshURL: `xxxx`,
	},
	expectError: `logic token refresh URL "xxxx" not valid`,
}, {
	about: "invalid query tracing value",
	config: controller.Config{
		controller.QueryTracingEnabled: "invalid",
	},
	expectError: `query-tracing-enabled: expected bool, got string\("invalid"\)`,
}, {
	about: "invalid query tracing threshold value",
	config: controller.Config{
		controller.QueryTracingThreshold: "invalid",
	},
	expectError: `query-tracing-threshold: conversion to duration: time: invalid duration "invalid"`,
}, {
	about: "negative query tracing threshold duration",
	config: controller.Config{
		controller.QueryTracingThreshold: "-1s",
	},
	expectError: `query-tracing-threshold value "-1s" must be a positive duration`,
}, {
	about: "invalid open telemetry tracing enabled value",
	config: controller.Config{
		controller.OpenTelemetryEnabled: "invalid",
	},
	expectError: `open-telemetry-enabled: expected bool, got string\("invalid"\)`,
}, {
	about: "invalid open telemetry tracing insecure value",
	config: controller.Config{
		controller.OpenTelemetryInsecure: "invalid",
	},
	expectError: `open-telemetry-insecure: expected bool, got string\("invalid"\)`,
}, {
	about: "invalid open telemetry tracing stack traces value",
	config: controller.Config{
		controller.OpenTelemetryStackTraces: "invalid",
	},
	expectError: `open-telemetry-stack-traces: expected bool, got string\("invalid"\)`,
}, {
	about: "invalid open telemetry tracing sample ratio value",
	config: controller.Config{
		controller.OpenTelemetrySampleRatio: "invalid",
	},
	expectError: `open-telemetry-sample-ratio: strconv.ParseFloat: parsing "invalid": invalid syntax`,
}, {
	about: "invalid open telemetry tracing tail sampling threshold value",
	config: controller.Config{
		controller.OpenTelemetryTailSamplingThreshold: "invalid",
	},
	expectError: `open-telemetry-tail-sampling-threshold: conversion to duration: time: invalid duration "invalid"`,
}, {
	about: "invalid object store type value",
	config: controller.Config{
		controller.ObjectStoreType: "invalid",
	},
	expectError: `invalid object store type "invalid" not valid`,
}, {
	about: "invalid object store type type",
	config: controller.Config{
		controller.ObjectStoreType: 1,
	},
	expectError: `object-store-type: expected string, got int\(1\)`,
}, {
	about: "invalid object store s3 endpoint value",
	config: controller.Config{
		controller.ObjectStoreS3Endpoint: 1,
	},
	expectError: `object-store-s3-endpoint: expected string, got int\(1\)`,
}, {
	about: "invalid object store s3 static key value",
	config: controller.Config{
		controller.ObjectStoreS3StaticKey: 1,
	},
	expectError: `object-store-s3-static-key: expected string, got int\(1\)`,
}, {
	about: "invalid object store s3 static secret value",
	config: controller.Config{
		controller.ObjectStoreS3StaticSecret: 1,
	},
	expectError: `object-store-s3-static-secret: expected string, got int\(1\)`,
}, {
	about: "invalid object store s3 static session value",
	config: controller.Config{
		controller.ObjectStoreS3StaticSession: 1,
	},
	expectError: `object-store-s3-static-session: expected string, got int\(1\)`,
}, {
	about: "invalid jujud-controller-snap-source value",
	config: controller.Config{
		controller.JujudControllerSnapSource: "latest/stable",
	},
	expectError: `jujud-controller-snap-source value "latest/stable" must be one of legacy, snapstore, local or local-dangerous.`,
}, {
	about: "empty controller name",
	config: controller.Config{
		controller.ControllerName: "",
	},
	expectError: `controller-name: expected non-empty controller-name.*`,
}, {
	about: "invalid controller name",
	config: controller.Config{
		controller.ControllerName: "is_invalid",
	},
	expectError: `controller-name value must be a valid controller name.*`,
}, {
	about: "invalid ssh port",
	config: controller.Config{
		controller.SSHServerPort: 0,
	},
	expectError: `non-positive integer for ssh-server-port not valid`,
}, {
	about: "SSH port equals api server port",
	config: controller.Config{
		controller.APIPort:       17070,
		controller.SSHServerPort: 17070,
	},
	expectError: `ssh-server-port matching api-port not valid`,
}}

func (s *ConfigSuite) TestNewConfig(c *tc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %v", i, test.about)
		_, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, test.config)
		if test.expectError != "" {
			c.Check(err, tc.ErrorMatches, test.expectError)
		} else {
			c.Check(err, tc.ErrorIsNil)
		}
	}
}

func (s *ConfigSuite) TestResourceDownloadLimits(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"application-resource-download-limit": "42",
			"controller-resource-download-limit":  "666",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.ApplicationResourceDownloadLimit(), tc.Equals, 42)
	c.Assert(cfg.ControllerResourceDownloadLimit(), tc.Equals, 666)
}

func (s *ConfigSuite) TestTxnLogConfigDefault(c *tc.C) {
	cfg, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.MaxTxnLogSizeMB(), tc.Equals, 10)
}

func (s *ConfigSuite) TestTxnLogConfigValue(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"max-txn-log-size": "8G",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.MaxTxnLogSizeMB(), tc.Equals, 8192)
}

func (s *ConfigSuite) TestMaxPruneTxnConfigDefault(c *tc.C) {
	cfg, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.MaxPruneTxnBatchSize(), tc.Equals, 1*1000*1000)
	c.Check(cfg.MaxPruneTxnPasses(), tc.Equals, 100)
}

func (s *ConfigSuite) TestMaxPruneTxnConfigValue(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"max-prune-txn-batch-size": "12345678",
			"max-prune-txn-passes":     "10",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.MaxPruneTxnBatchSize(), tc.Equals, 12345678)
	c.Check(cfg.MaxPruneTxnPasses(), tc.Equals, 10)
}

func (s *ConfigSuite) TestPruneTxnQueryCount(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"prune-txn-query-count": "500",
			"prune-txn-sleep-time":  "5ms",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.PruneTxnQueryCount(), tc.Equals, 500)
	c.Check(cfg.PruneTxnSleepTime(), tc.Equals, 5*time.Millisecond)
}

func (s *ConfigSuite) TestPublicDNSAddressConfigValue(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"public-dns-address": "controller.test.com:12345",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.PublicDNSAddress(), tc.Equals, "controller.test.com:12345")
}

func (s *ConfigSuite) TestNetworkSpaceConfigValues(c *tc.C) {
	managementSpace := network.SpaceName("space2")

	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.JujuManagementSpace: managementSpace,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.JujuManagementSpace(), tc.Equals, managementSpace)
}

func (s *ConfigSuite) TestNetworkSpaceConfigDefaults(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.JujuManagementSpace(), tc.Equals, network.SpaceName(""))
}

func (s *ConfigSuite) TestAuditLogDefaults(c *tc.C) {
	cfg, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.AuditingEnabled(), tc.Equals, true)
	c.Assert(cfg.AuditLogCaptureArgs(), tc.Equals, false)
	c.Assert(cfg.AuditLogMaxSizeMB(), tc.Equals, 300)
	c.Assert(cfg.AuditLogMaxBackups(), tc.Equals, 10)
	c.Assert(cfg.AuditLogExcludeMethods(), tc.DeepEquals,
		set.NewStrings(controller.DefaultAuditLogExcludeMethods))
}

func (s *ConfigSuite) TestAuditLogValues(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"auditing-enabled":          false,
			"audit-log-capture-args":    true,
			"audit-log-max-size":        "100M",
			"audit-log-max-backups":     10.0,
			"audit-log-exclude-methods": "Fleet.Foxes,King.Gizzard,ReadOnlyMethods",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.AuditingEnabled(), tc.Equals, false)
	c.Assert(cfg.AuditLogCaptureArgs(), tc.Equals, true)
	c.Assert(cfg.AuditLogMaxSizeMB(), tc.Equals, 100)
	c.Assert(cfg.AuditLogMaxBackups(), tc.Equals, 10)
	c.Assert(cfg.AuditLogExcludeMethods(), tc.DeepEquals, set.NewStrings(
		"Fleet.Foxes",
		"King.Gizzard",
		"ReadOnlyMethods",
	))
}

func (s *ConfigSuite) TestAuditLogExcludeMethodsType(c *tc.C) {
	_, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"audit-log-exclude-methods": []int{2, 3, 4},
		},
	)
	c.Assert(err, tc.ErrorMatches, `audit-log-exclude-methods: expected string, got .*`)
}

func (s *ConfigSuite) TestAuditLogFloatBackupsLoadedDirectly(c *tc.C) {
	// We still need to be able to handle floats in data loaded from the DB.
	cfg := controller.Config{
		controller.AuditLogMaxBackups: 10.0,
	}
	c.Assert(cfg.AuditLogMaxBackups(), tc.Equals, 10)
}

func (s *ConfigSuite) TestConfigAllSpacesAsMergedConstraints(c *tc.C) {
	managementSpace := "management-space"
	constraintSpace := "constraint-space"

	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.JujuManagementSpace: managementSpace,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	got := *cfg.AsSpaceConstraints(&[]string{constraintSpace})
	c.Check(got, tc.DeepEquals, []string{constraintSpace, managementSpace})
}

func (s *ConfigSuite) TestConfigNoSpacesNilSpaceConfigPreserved(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.AsSpaceConstraints(nil), tc.IsNil)
}

func (s *ConfigSuite) TestCAASImageRepo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Ensure no requests are made from controller config code.
	mockRoundTripper := mocks.NewMockRoundTripper(ctrl)
	s.PatchValue(&registry.DefaultTransport, mockRoundTripper)

	type test struct {
		content  string
		expected string
	}
	for _, imageRepo := range []test{
		//used to reset since we don't have a --reset option
		{content: "", expected: ""},
		{content: "docker.io/juju-operator-repo", expected: ""},
		{content: "registry.foo.com/jujuqa", expected: ""},
		{content: "ghcr.io/jujuqa", expected: ""},
		{content: "registry.gitlab.com/jujuqa", expected: ""},
		{
			content: fmt.Sprintf(`
{
    "serveraddress": "ghcr.io",
    "auth": "%s",
    "repository": "ghcr.io/test-account"
}`, base64.StdEncoding.EncodeToString([]byte("username:pwd"))),
			expected: "ghcr.io/test-account"},
	} {
		c.Logf("testing %#v", imageRepo)
		if imageRepo.expected == "" {
			imageRepo.expected = imageRepo.content
		}
		cfg, err := controller.NewConfig(
			testing.ControllerTag.Id(),
			testing.CACert,
			map[string]interface{}{
				controller.CAASImageRepo: imageRepo.content,
			},
		)
		c.Check(err, tc.ErrorIsNil)
		imageRepoDetails, err := docker.NewImageRepoDetails(cfg.CAASImageRepo())
		c.Check(err, tc.ErrorIsNil)
		c.Check(imageRepoDetails.Repository, tc.Equals, imageRepo.expected)
	}
}

func (s *ConfigSuite) TestControllerNameDefault(c *tc.C) {
	cfg := controller.Config{}
	c.Check(cfg.ControllerName(), tc.Equals, "")
}

func (s *ConfigSuite) TestControllerNameSetGet(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.ControllerName: "test",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.ControllerName(), tc.Equals, "test")
}

func (s *ConfigSuite) TestMaxDebugLogDuration(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"max-debug-log-duration": "90m",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.MaxDebugLogDuration(), tc.Equals, 90*time.Minute)
}

func (s *ConfigSuite) TestMaxDebugLogDurationSchemaCoerce(c *tc.C) {
	_, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"max-debug-log-duration": "12",
		},
	)
	c.Assert(err, tc.ErrorMatches, `max-debug-log-duration: conversion to duration: time: missing unit in duration "?12"?`)
}

func (s *ConfigSuite) TestFeatureFlags(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.Features: "foo,bar",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.Features().Values(), tc.SameContents, []string{"foo", "bar"})
}

func (s *ConfigSuite) TestDefaults(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.AgentRateLimitMax(), tc.Equals, controller.DefaultAgentRateLimitMax)
	c.Assert(cfg.AgentRateLimitRate(), tc.Equals, controller.DefaultAgentRateLimitRate)
	c.Assert(cfg.MaxDebugLogDuration(), tc.Equals, controller.DefaultMaxDebugLogDuration)
	c.Assert(cfg.AgentLogfileMaxBackups(), tc.Equals, controller.DefaultAgentLogfileMaxBackups)
	c.Assert(cfg.AgentLogfileMaxSizeMB(), tc.Equals, controller.DefaultAgentLogfileMaxSize)
	c.Assert(cfg.ModelLogfileMaxBackups(), tc.Equals, controller.DefaultModelLogfileMaxBackups)
	c.Assert(cfg.ModelLogfileMaxSizeMB(), tc.Equals, controller.DefaultModelLogfileMaxSize)
	c.Assert(cfg.ApplicationResourceDownloadLimit(), tc.Equals, controller.DefaultApplicationResourceDownloadLimit)
	c.Assert(cfg.ControllerResourceDownloadLimit(), tc.Equals, controller.DefaultControllerResourceDownloadLimit)
	c.Assert(cfg.QueryTracingEnabled(), tc.Equals, controller.DefaultQueryTracingEnabled)
	c.Assert(cfg.QueryTracingThreshold(), tc.Equals, controller.DefaultQueryTracingThreshold)
	c.Assert(cfg.SSHServerPort(), tc.Equals, controller.DefaultSSHServerPort)
	c.Assert(cfg.SSHMaxConcurrentConnections(), tc.Equals, controller.DefaultSSHMaxConcurrentConnections)
}

func (s *ConfigSuite) TestAgentLogfile(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"agent-logfile-max-size":    "35M",
			"agent-logfile-max-backups": "17",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.AgentLogfileMaxBackups(), tc.Equals, 17)
	c.Assert(cfg.AgentLogfileMaxSizeMB(), tc.Equals, 35)
}

func (s *ConfigSuite) TestAgentLogfileBackupErr(c *tc.C) {
	_, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"agent-logfile-max-backups": "two",
		},
	)
	c.Assert(err.Error(), tc.Equals, `agent-logfile-max-backups: expected number, got string("two")`)
}

func (s *ConfigSuite) TestModelLogfile(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"model-logfile-max-size":    "25M",
			"model-logfile-max-backups": "15",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.ModelLogfileMaxBackups(), tc.Equals, 15)
	c.Assert(cfg.ModelLogfileMaxSizeMB(), tc.Equals, 25)
}

func (s *ConfigSuite) TestModelLogfileBackupErr(c *tc.C) {
	_, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"model-logfile-max-backups": "two",
		},
	)
	c.Assert(err.Error(), tc.Equals, `model-logfile-max-backups: expected number, got string("two")`)
}

func (s *ConfigSuite) TestAgentRateLimitMax(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"agent-ratelimit-max": "0",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.AgentRateLimitMax(), tc.Equals, 0)
}

func (s *ConfigSuite) TestAgentRateLimitRate(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.AgentRateLimitRate(), tc.Equals, controller.DefaultAgentRateLimitRate)

	cfg[controller.AgentRateLimitRate] = time.Second
	c.Assert(cfg.AgentRateLimitRate(), tc.Equals, time.Second)

	cfg[controller.AgentRateLimitRate] = "500ms"
	c.Assert(cfg.AgentRateLimitRate(), tc.Equals, 500*time.Millisecond)
}

func (s *ConfigSuite) TestJujuDBSnapChannel(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.JujuDBSnapChannel(), tc.Equals, controller.DefaultJujuDBSnapChannel)

	cfg, err = controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"juju-db-snap-channel": "latest/candidate",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.JujuDBSnapChannel(), tc.Equals, "latest/candidate")
}

func (s *ConfigSuite) TestMigrationMinionWaitMax(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cfg.MigrationMinionWaitMax(), tc.Equals, controller.DefaultMigrationMinionWaitMax)

	cfg[controller.MigrationMinionWaitMax] = "500ms"
	c.Assert(cfg.MigrationMinionWaitMax(), tc.Equals, 500*time.Millisecond)
}

func (s *ConfigSuite) TestQueryTraceEnabled(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cfg.QueryTracingEnabled(), tc.Equals, controller.DefaultQueryTracingEnabled)

	cfg[controller.QueryTracingEnabled] = true
	c.Assert(cfg.QueryTracingEnabled(), tc.Equals, true)
}

func (s *ConfigSuite) TestQueryTraceThreshold(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cfg.QueryTracingThreshold(), tc.Equals, controller.DefaultQueryTracingThreshold)

	cfg[controller.QueryTracingThreshold] = time.Second * 10
	c.Assert(cfg.QueryTracingThreshold(), tc.Equals, time.Second*10)

	d := time.Second * 10
	cfg[controller.QueryTracingThreshold] = d.String()

	bytes, err := json.Marshal(cfg)
	c.Assert(err, tc.ErrorIsNil)

	var cfg2 controller.Config
	err = json.Unmarshal(bytes, &cfg2)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cfg2.QueryTracingThreshold(), tc.Equals, time.Second*10)
}

func (s *ConfigSuite) TestOpenTelemetryEnabled(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cfg.OpenTelemetryEnabled(), tc.Equals, controller.DefaultOpenTelemetryEnabled)

	cfg[controller.OpenTelemetryEnabled] = true
	c.Assert(cfg.OpenTelemetryEnabled(), tc.Equals, true)
}

func (s *ConfigSuite) TestOpenTelemetryInsecure(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cfg.OpenTelemetryInsecure(), tc.Equals, controller.DefaultOpenTelemetryInsecure)

	cfg[controller.OpenTelemetryInsecure] = true
	c.Assert(cfg.OpenTelemetryInsecure(), tc.Equals, true)
}

func (s *ConfigSuite) TestOpenTelemetryStackTraces(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cfg.OpenTelemetryStackTraces(), tc.Equals, controller.DefaultOpenTelemetryStackTraces)

	cfg[controller.OpenTelemetryStackTraces] = true
	c.Assert(cfg.OpenTelemetryStackTraces(), tc.Equals, true)
}

func (s *ConfigSuite) TestOpenTelemetryEndpointSettingValue(c *tc.C) {
	mURL := "http://meshuggah.com/endpoint"
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.OpenTelemetryEndpoint: mURL,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.OpenTelemetryEndpoint(), tc.Equals, mURL)
}

func (s *ConfigSuite) TestOpenTelemetrySampleRatio(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cfg.OpenTelemetrySampleRatio(), tc.Equals, controller.DefaultOpenTelemetrySampleRatio)

	cfg[controller.OpenTelemetrySampleRatio] = 0.42
	c.Assert(cfg.OpenTelemetrySampleRatio(), tc.Equals, 0.42)
}

func (s *ConfigSuite) TestOpenTelemetryTailSamplingThreshold(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cfg.OpenTelemetryTailSamplingThreshold(), tc.Equals, controller.DefaultOpenTelemetryTailSamplingThreshold)

	cfg[controller.OpenTelemetryTailSamplingThreshold] = "1s"
	c.Assert(cfg.OpenTelemetryTailSamplingThreshold(), tc.Equals, time.Second)
}

func (s *ConfigSuite) TestSSHServerPort(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.SSHServerPort: 10,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.SSHServerPort(), tc.Equals, 10)
}

func (s *ConfigSuite) TestSSHServerConcurrentConnections(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.SSHMaxConcurrentConnections: 10,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.SSHMaxConcurrentConnections(), tc.Equals, 10)
}

func (s *ConfigSuite) TestObjectStoreType(c *tc.C) {
	backendType := "file"
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.ObjectStoreType: backendType,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.ObjectStoreType(), tc.Equals, objectstore.FileBackend)
}

func (s *ConfigSuite) TestObjectStoreS3Endpoint(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.ObjectStoreS3Endpoint: "http://localhost:9000",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.ObjectStoreS3Endpoint(), tc.Equals, "http://localhost:9000")
}

func (s *ConfigSuite) TestObjectStoreS3Credentials(c *tc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.ObjectStoreS3StaticKey:     "key",
			controller.ObjectStoreS3StaticSecret:  "secret",
			controller.ObjectStoreS3StaticSession: "session",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.ObjectStoreS3StaticKey(), tc.Equals, "key")
	c.Assert(cfg.ObjectStoreS3StaticSecret(), tc.Equals, "secret")
	c.Assert(cfg.ObjectStoreS3StaticSession(), tc.Equals, "session")
}
