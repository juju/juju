// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/loggo"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	utilscert "github.com/juju/utils/cert"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cert"
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

func (s *ConfigSuite) TestGenerateControllerCertAndKey(c *gc.C) {
	// Add a cert.
	s.FakeHomeSuite.Home.AddFiles(c, gitjujutesting.TestFile{Name: ".ssh/id_rsa.pub", Data: "rsa\n"})

	for _, test := range []struct {
		caCert    string
		caKey     string
		sanValues []string
	}{{
		caCert: testing.CACert,
		caKey:  testing.CAKey,
	}, {
		caCert:    testing.CACert,
		caKey:     testing.CAKey,
		sanValues: []string{"10.0.0.1", "192.168.1.1"},
	}} {
		certPEM, keyPEM, err := controller.GenerateControllerCertAndKey(test.caCert, test.caKey, test.sanValues)
		c.Assert(err, jc.ErrorIsNil)

		_, _, err = utilscert.ParseCertAndKey(certPEM, keyPEM)
		c.Check(err, jc.ErrorIsNil)

		err = cert.Verify(certPEM, testing.CACert, time.Now())
		c.Assert(err, jc.ErrorIsNil)
		err = cert.Verify(certPEM, testing.CACert, time.Now().AddDate(9, 0, 0))
		c.Assert(err, jc.ErrorIsNil)
		err = cert.Verify(certPEM, testing.CACert, time.Now().AddDate(10, 0, 1))
		c.Assert(err, gc.NotNil)
		srvCert, err := utilscert.ParseCert(certPEM)
		c.Assert(err, jc.ErrorIsNil)
		sanIPs := make([]string, len(srvCert.IPAddresses))
		for i, ip := range srvCert.IPAddresses {
			sanIPs[i] = ip.String()
		}
		c.Assert(sanIPs, jc.SameContents, test.sanValues)
	}
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
	expectError: `bad CA certificate in configuration: no certificates found`,
}, {
	about: "bad controller UUID",
	config: controller.Config{
		controller.ControllerUUIDKey: "xxx",
		controller.CACertKey:         testing.CACert,
	},
	expectError: `controller-uuid: expected UUID, got string\("xxx"\)`,
}, {
	about: "HTTPS identity URL OK",
	config: controller.Config{
		controller.IdentityURL: "https://0.1.2.3/foo",
		controller.CACertKey:   testing.CACert,
	},
}, {
	about: "HTTP identity URL requires public key",
	config: controller.Config{
		controller.IdentityURL: "http://0.1.2.3/foo",
		controller.CACertKey:   testing.CACert,
	},
	expectError: `URL needs to be https when identity-public-key not provided`,
}, {
	about: "HTTP identity URL OK if public key is provided",
	config: controller.Config{
		controller.IdentityPublicKey: `o/yOqSNWncMo1GURWuez/dGR30TscmmuIxgjztpoHEY=`,
		controller.IdentityURL:       "http://0.1.2.3/foo",
		controller.CACertKey:         testing.CACert,
	},
}, {
	about: "invalid identity public key",
	config: controller.Config{
		controller.IdentityPublicKey: `xxxx`,
		controller.CACertKey:         testing.CACert,
	},
	expectError: `invalid identity public key: wrong length for base64 key, got 3 want 32`,
}, {
	about: "invalid management space name - whitespace",
	config: controller.Config{
		controller.CACertKey:           testing.CACert,
		controller.JujuManagementSpace: " ",
	},
	expectError: `juju mgmt space name " " not valid`,
}, {
	about: "invalid management space name - caps",
	config: controller.Config{
		controller.CACertKey:           testing.CACert,
		controller.JujuManagementSpace: "CAPS",
	},
	expectError: `juju mgmt space name "CAPS" not valid`,
}, {
	about: "invalid management space name - carriage return",
	config: controller.Config{
		controller.CACertKey:           testing.CACert,
		controller.JujuManagementSpace: "\n",
	},
	expectError: `juju mgmt space name "\\n" not valid`,
}, {
	about: "invalid HA space name - number",
	config: controller.Config{
		controller.CACertKey:   testing.CACert,
		controller.JujuHASpace: 666,
	},
	expectError: `type for juju HA space name 666 not valid`,
}, {
	about: "invalid HA space name - bool",
	config: controller.Config{
		controller.CACertKey:   testing.CACert,
		controller.JujuHASpace: true,
	},
	expectError: `type for juju HA space name true not valid`,
}, {
	about: "invalid audit log max size",
	config: controller.Config{
		controller.CACertKey:       testing.CACert,
		controller.AuditLogMaxSize: "abcd",
	},
	expectError: `invalid audit log max size in configuration: expected a non-negative number, got "abcd"`,
}, {
	about: "zero audit log max size",
	config: controller.Config{
		controller.CACertKey:       testing.CACert,
		controller.AuditingEnabled: true,
		controller.AuditLogMaxSize: "0M",
	},
	expectError: `invalid audit log max size: can't be 0 if auditing is enabled`,
}, {
	about: "invalid audit log max backups",
	config: controller.Config{
		controller.CACertKey:          testing.CACert,
		controller.AuditLogMaxBackups: -10,
	},
	expectError: `invalid audit log max backups: should be a number of files \(or 0 to keep all\), got -10`,
}, {
	about: "invalid audit log exclude",
	config: controller.Config{
		controller.CACertKey:              testing.CACert,
		controller.AuditLogExcludeMethods: []interface{}{"Dap.Kings", "ReadOnlyMethods", "Sharon Jones"},
	},
	expectError: `invalid audit log exclude methods: should be a list of "Facade.Method" names \(or "ReadOnlyMethods"\), got "Sharon Jones" at position 3`,
}}

func (s *ConfigSuite) TestValidate(c *gc.C) {
	for i, test := range validateTests {
		c.Logf("test %d: %v", i, test.about)
		err := test.config.Validate()
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

func (s *ConfigSuite) TestLogConfigDefaults(c *gc.C) {
	cfg, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.MaxLogsAge(), gc.Equals, 72*time.Hour)
	c.Assert(cfg.MaxLogSizeMB(), gc.Equals, 4096)
}

func (s *ConfigSuite) TestLogConfigValues(c *gc.C) {
	cfg, err := controller.NewConfig(
		testing.ControllerTag.Id(),
		testing.CACert,
		map[string]interface{}{
			"max-logs-size": "8G",
			"max-logs-age":  "96h",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.MaxLogsAge(), gc.Equals, 96*time.Hour)
	c.Assert(cfg.MaxLogSizeMB(), gc.Equals, 8192)
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
