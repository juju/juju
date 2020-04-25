// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/charmrepo/v5/csclient"
	"github.com/juju/collections/set"
	"github.com/juju/loggo"
	"github.com/juju/romulus"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type ConfigSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	home string
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *gc.C) {
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

func (s *ConfigSuite) TestValidate(c *gc.C) {
	// Normally Validate is only called as part of the NewConfig call, which
	// also does schema coercing. The NewConfig method takes the controller uuid
	// and cacert as separate args, so to get invalid ones, we skip that part.
	for i, test := range validateTests {
		c.Logf("test %d: %v", i, test.about)
		err := test.config.Validate()
		if test.expectError != "" {
			c.Check(err, gc.ErrorMatches, test.expectError)
		} else {
			c.Check(err, jc.ErrorIsNil)
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
	about: "invalid HA space name - number",
	config: controller.Config{
		controller.JujuHASpace: 666,
	},
	expectError: `juju-ha-space: expected string, got int\(666\)`,
}, {
	about: "invalid HA space name - bool",
	config: controller.Config{
		controller.JujuHASpace: true,
	},
	expectError: `juju-ha-space: expected string, got bool\(true\)`,
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
		controller.AuditLogExcludeMethods: []interface{}{"Dap.Kings", "ReadOnlyMethods", "Sharon Jones"},
	},
	expectError: `invalid audit log exclude methods: should be a list of "Facade.Method" names \(or "ReadOnlyMethods"\), got "Sharon Jones" at position 3`,
}, {
	about: "invalid model log max size",
	config: controller.Config{
		controller.ModelLogsSize: "abcd",
	},
	expectError: `invalid model logs size in configuration: expected a non-negative number, got "abcd"`,
}, {
	about: "zero model log max size",
	config: controller.Config{
		controller.ModelLogsSize: "0",
	},
	expectError: "model logs size less than 1 MB not valid",
}, {
	about: "invalid CAAS docker image repo",
	config: controller.Config{
		controller.CAASImageRepo: "foo?bar",
	},
	expectError: `docker image path "foo\?bar" not valid`,
}, {
	about: "invalid CAAS operator docker image repo - leading colon",
	config: controller.Config{
		controller.CAASImageRepo: ":foo",
	},
	expectError: `docker image path ":foo" not valid`,
}, {
	about: "invalid CAAS docker image repo - trailing colon",
	config: controller.Config{
		controller.CAASImageRepo: "foo:",
	},
	expectError: `docker image path "foo:" not valid`,
}, {
	about: "invalid CAAS docker image repo - extra colon",
	config: controller.Config{
		controller.CAASImageRepo: "foo::bar",
	},
	expectError: `docker image path "foo::bar" not valid`,
}, {
	about: "invalid CAAS docker image repo - leading /",
	config: controller.Config{
		controller.CAASImageRepo: "/foo",
	},
	expectError: `docker image path "/foo" not valid`,
}, {
	about: "invalid CAAS docker image repo - extra /",
	config: controller.Config{
		controller.CAASImageRepo: "foo//bar",
	},
	expectError: `docker image path "foo//bar" not valid`,
}, {
	about: "negative controller-api-port",
	config: controller.Config{
		controller.ControllerAPIPort: -5,
	},
	expectError: `non-positive integer for controller-api-port not valid`,
}, {
	about: "controller-api-port matching api-port",
	config: controller.Config{
		controller.APIPort:           12345,
		controller.ControllerAPIPort: 12345,
	},
	expectError: `controller-api-port matching api-port not valid`,
}, {
	about: "controller-api-port matching state-port",
	config: controller.Config{
		controller.APIPort:           12345,
		controller.StatePort:         54321,
		controller.ControllerAPIPort: 54321,
	},
	expectError: `controller-api-port matching state-port not valid`,
}, {
	about: "api-port-open-delay not a duration",
	config: controller.Config{
		controller.APIPortOpenDelay: "15",
	},
	expectError: `api-port-open-delay value "15" must be a valid duration`,
}, {
	about: "txn-prune-sleep-time not a duration",
	config: controller.Config{
		controller.PruneTxnSleepTime: "15",
	},
	expectError: `prune-txn-sleep-time must be a valid duration \(eg "10ms"\): time: missing unit in duration 15`,
}, {
	about: "mongo-memory-profile not valid",
	config: controller.Config{
		controller.MongoMemoryProfile: "not-valid",
	},
	expectError: `mongo-memory-profile: expected one of "low" or "default" got string\("not-valid"\)`,
}, {
	about: "max-debug-log-duration not valid",
	config: controller.Config{
		controller.MaxDebugLogDuration: time.Duration(0),
	},
	expectError: `max-debug-log-duration cannot be zero`,
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
	expectError: `agent-ratelimit-rate: conversion to duration: time: missing unit in duration 150`,
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
}, {}}

func (s *ConfigSuite) TestNewConfig(c *gc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %v", i, test.about)
		_, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, test.config)
		if test.expectError != "" {
			c.Check(err, gc.ErrorMatches, test.expectError)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
	}
}

func (s *ConfigSuite) TestAPIPortDefaults(c *gc.C) {
	cfg, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.APIPortOpenDelay(), gc.Equals, 2*time.Second)
}

func (s *ConfigSuite) TestLogConfigDefaults(c *gc.C) {
	cfg, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.ModelLogsSizeMB(), gc.Equals, 20)
}

func (s *ConfigSuite) TestLogConfigValues(c *gc.C) {
	c.Assert(controller.AllowedUpdateConfigAttributes.Contains(controller.ModelLogsSize), jc.IsTrue)

	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"max-logs-size":   "8G",
			"max-logs-age":    "96h",
			"model-logs-size": "35M",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.ModelLogsSizeMB(), gc.Equals, 35)
}

func (s *ConfigSuite) TestTxnLogConfigDefault(c *gc.C) {
	cfg, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.MaxTxnLogSizeMB(), gc.Equals, 10)
}

func (s *ConfigSuite) TestTxnLogConfigValue(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"max-txn-log-size": "8G",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.MaxTxnLogSizeMB(), gc.Equals, 8192)
}

func (s *ConfigSuite) TestMaxPruneTxnConfigDefault(c *gc.C) {
	cfg, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg.MaxPruneTxnBatchSize(), gc.Equals, 1*1000*1000)
	c.Check(cfg.MaxPruneTxnPasses(), gc.Equals, 100)
}

func (s *ConfigSuite) TestMaxPruneTxnConfigValue(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"max-prune-txn-batch-size": "12345678",
			"max-prune-txn-passes":     "10",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg.MaxPruneTxnBatchSize(), gc.Equals, 12345678)
	c.Check(cfg.MaxPruneTxnPasses(), gc.Equals, 10)
}

func (s *ConfigSuite) TestPruneTxnQueryCount(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"prune-txn-query-count": "500",
			"prune-txn-sleep-time":  "5ms",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg.PruneTxnQueryCount(), gc.Equals, 500)
	c.Check(cfg.PruneTxnSleepTime(), gc.Equals, 5*time.Millisecond)
}

func (s *ConfigSuite) TestNetworkSpaceConfigValues(c *gc.C) {
	haSpace := "space1"
	managementSpace := "space2"

	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.JujuHASpace:         haSpace,
			controller.JujuManagementSpace: managementSpace,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.JujuHASpace(), gc.Equals, haSpace)
	c.Assert(cfg.JujuManagementSpace(), gc.Equals, managementSpace)
}

func (s *ConfigSuite) TestNetworkSpaceConfigDefaults(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.JujuHASpace(), gc.Equals, "")
	c.Assert(cfg.JujuManagementSpace(), gc.Equals, "")
}

func (s *ConfigSuite) TestAuditLogDefaults(c *gc.C) {
	cfg, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AuditingEnabled(), gc.Equals, true)
	c.Assert(cfg.AuditLogCaptureArgs(), gc.Equals, false)
	c.Assert(cfg.AuditLogMaxSizeMB(), gc.Equals, 300)
	c.Assert(cfg.AuditLogMaxBackups(), gc.Equals, 10)
	c.Assert(cfg.AuditLogExcludeMethods(), gc.DeepEquals,
		set.NewStrings(controller.DefaultAuditLogExcludeMethods...))
}

func (s *ConfigSuite) TestAuditLogValues(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"auditing-enabled":          false,
			"audit-log-capture-args":    true,
			"audit-log-max-size":        "100M",
			"audit-log-max-backups":     10.0,
			"audit-log-exclude-methods": []string{"Fleet.Foxes", "King.Gizzard", "ReadOnlyMethods"},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AuditingEnabled(), gc.Equals, false)
	c.Assert(cfg.AuditLogCaptureArgs(), gc.Equals, true)
	c.Assert(cfg.AuditLogMaxSizeMB(), gc.Equals, 100)
	c.Assert(cfg.AuditLogMaxBackups(), gc.Equals, 10)
	c.Assert(cfg.AuditLogExcludeMethods(), gc.DeepEquals, set.NewStrings(
		"Fleet.Foxes",
		"King.Gizzard",
		"ReadOnlyMethods",
	))
}

func (s *ConfigSuite) TestAuditLogExcludeMethodsType(c *gc.C) {
	_, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"audit-log-exclude-methods": []int{2, 3, 4},
		},
	)
	c.Assert(err, gc.ErrorMatches, `audit-log-exclude-methods\[0\]: expected string, got int\(2\)`)
}

func (s *ConfigSuite) TestAuditLogFloatBackupsLoadedDirectly(c *gc.C) {
	// We still need to be able to handle floats in data loaded from the DB.
	cfg := controller.Config{
		controller.AuditLogMaxBackups: 10.0,
	}
	c.Assert(cfg.AuditLogMaxBackups(), gc.Equals, 10)
}

func (s *ConfigSuite) TestConfigManagementSpaceAsConstraint(c *gc.C) {
	managementSpace := "management-space"
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{controller.JujuHASpace: managementSpace},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*cfg.AsSpaceConstraints(nil), gc.DeepEquals, []string{managementSpace})
}

func (s *ConfigSuite) TestConfigHASpaceAsConstraint(c *gc.C) {
	haSpace := "ha-space"
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{controller.JujuHASpace: haSpace},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*cfg.AsSpaceConstraints(nil), gc.DeepEquals, []string{haSpace})
}

func (s *ConfigSuite) TestConfigAllSpacesAsMergedConstraints(c *gc.C) {
	haSpace := "ha-space"
	managementSpace := "management-space"
	constraintSpace := "constraint-space"

	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.JujuHASpace:         haSpace,
			controller.JujuManagementSpace: managementSpace,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	got := *cfg.AsSpaceConstraints(&[]string{constraintSpace})
	c.Check(got, gc.DeepEquals, []string{constraintSpace, haSpace, managementSpace})
}

func (s *ConfigSuite) TestConfigNoSpacesNilSpaceConfigPreserved(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg.AsSpaceConstraints(nil), gc.IsNil)
}

func (s *ConfigSuite) TestCAASImageRepo(c *gc.C) {
	for _, imageRepo := range []string{
		"", //used to reset since we don't have a --reset option
		"juju-operator-repo",
		"registry.foo.com",
		"registry.foo.com/me",
	} {
		cfg, err := controller.NewConfig(
			testing.ControllerTag.Id(),
			testing.CACert,
			map[string]interface{}{
				controller.CAASImageRepo: imageRepo,
			},
		)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cfg.CAASImageRepo(), gc.Equals, imageRepo)
	}
}

func (s *ConfigSuite) TestCharmstoreURLDefault(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg.CharmStoreURL(), gc.Equals, csclient.ServerURL)
}

func (s *ConfigSuite) TestControllerNameDefault(c *gc.C) {
	cfg := controller.Config{}
	c.Check(cfg.ControllerName(), gc.Equals, "")
}

func (s *ConfigSuite) TestControllerNameSetGet(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.ControllerName: "test",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg.ControllerName(), gc.Equals, "test")
}

func (s *ConfigSuite) TestCharmstoreURLSettingValue(c *gc.C) {
	csURL := "http://homestarrunner.com/charmstore"
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.CharmStoreURL: csURL,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.CharmStoreURL(), gc.Equals, csURL)
}

func (s *ConfigSuite) TestMeteringURLDefault(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg.MeteringURL(), gc.Equals, romulus.DefaultAPIRoot)
}

func (s *ConfigSuite) TestMeteringURLSettingValue(c *gc.C) {
	mURL := "http://homestarrunner.com/metering"
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			controller.MeteringURL: mURL,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.MeteringURL(), gc.Equals, mURL)
}

func (s *ConfigSuite) TestMaxDebugLogDuration(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"max-debug-log-duration": "90m",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.MaxDebugLogDuration(), gc.Equals, 90*time.Minute)
}

func (s *ConfigSuite) TestMaxDebugLogDurationSchemaCoerce(c *gc.C) {
	_, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"max-debug-log-duration": "12",
		},
	)
	c.Assert(err.Error(), gc.Equals, "max-debug-log-duration: conversion to duration: time: missing unit in duration 12")
}

func (s *ConfigSuite) TestDefaults(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AgentRateLimitMax(), gc.Equals, controller.DefaultAgentRateLimitMax)
	c.Assert(cfg.AgentRateLimitRate(), gc.Equals, controller.DefaultAgentRateLimitRate)
	c.Assert(cfg.MaxDebugLogDuration(), gc.Equals, controller.DefaultMaxDebugLogDuration)
	c.Assert(cfg.ModelLogfileMaxBackups(), gc.Equals, controller.DefaultModelLogfileMaxBackups)
	c.Assert(cfg.ModelLogfileMaxSizeMB(), gc.Equals, controller.DefaultModelLogfileMaxSize)
}

func (s *ConfigSuite) TestModelLogfile(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"model-logfile-max-size":    "25M",
			"model-logfile-max-backups": "15",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.ModelLogfileMaxBackups(), gc.Equals, 15)
	c.Assert(cfg.ModelLogfileMaxSizeMB(), gc.Equals, 25)
}

func (s *ConfigSuite) TestModelLogfileBackupErr(c *gc.C) {
	_, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"model-logfile-max-backups": "two",
		},
	)
	c.Assert(err.Error(), gc.Equals, `model-logfile-max-backups: expected number, got string("two")`)
}

func (s *ConfigSuite) TestAgentRateLimitMax(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"agent-ratelimit-max": "0",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AgentRateLimitMax(), gc.Equals, 0)
}

func (s *ConfigSuite) TestAgentRateLimitRate(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AgentRateLimitRate(), gc.Equals, controller.DefaultAgentRateLimitRate)

	cfg[controller.AgentRateLimitRate] = time.Second
	c.Assert(cfg.AgentRateLimitRate(), gc.Equals, time.Second)

	cfg[controller.AgentRateLimitRate] = "500ms"
	c.Assert(cfg.AgentRateLimitRate(), gc.Equals, 500*time.Millisecond)
}

func (s *ConfigSuite) TestJujuDBSnapChannel(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.JujuDBSnapChannel(), gc.Equals, controller.DefaultJujuDBSnapChannel)

	cfg, err = controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"juju-db-snap-channel": "latest/candidate",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.JujuDBSnapChannel(), gc.Equals, "latest/candidate")
}
