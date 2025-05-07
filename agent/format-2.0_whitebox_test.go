// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The format tests are white box tests, meaning that the tests are in the
// same package as the code, as all the format details are internal to the
// package.

package agent

import (
	"path/filepath"
	"time"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"

	agentconstants "github.com/juju/juju/agent/constants"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/testing"
)

type format_2_0Suite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&format_2_0Suite{})

func (s *format_2_0Suite) TestStatePortNotParsedWithoutSecret(c *tc.C) {
	dataDir := c.MkDir()
	configPath := filepath.Join(dataDir, agentconstants.AgentConfigFilename)
	err := utils.AtomicWriteFile(configPath, []byte(agentConfig2_0NotStateMachine), 0600)
	c.Assert(err, jc.ErrorIsNil)
	readConfig, err := ReadConfig(configPath)
	c.Assert(err, jc.ErrorIsNil)
	_, available := readConfig.StateServingInfo()
	c.Assert(available, jc.IsFalse)
}

func (*format_2_0Suite) TestReadConfWithExisting2_0ConfigFileContents(c *tc.C) {
	dataDir := c.MkDir()
	configPath := filepath.Join(dataDir, agentconstants.AgentConfigFilename)
	err := utils.AtomicWriteFile(configPath, []byte(agentConfig2_0Contents), 0600)
	c.Assert(err, jc.ErrorIsNil)

	config, err := ReadConfig(configPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(config.UpgradedToVersion(), jc.DeepEquals, semversion.MustParse("1.17.5.1"))
	c.Assert(config.Jobs(), jc.DeepEquals, []model.MachineJob{model.JobManageModel})
}

func (*format_2_0Suite) TestMarshalUnmarshal(c *tc.C) {
	loggingConfig := "juju=INFO;unit=INFO"
	config := newTestConfig(c)
	// configFilePath is not serialized as it is the location of the file.
	config.configFilePath = ""

	config.SetLoggingConfig(loggingConfig)

	data, err := format_2_0.marshal(config)
	c.Assert(err, jc.ErrorIsNil)
	newConfig, err := format_2_0.unmarshal(data)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(newConfig, tc.DeepEquals, config)
	c.Check(newConfig.LoggingConfig(), tc.Equals, loggingConfig)
}

func (*format_2_0Suite) TestQueryTracing(c *tc.C) {
	config := newTestConfig(c)
	// configFilePath is not serialized as it is the location of the file.
	config.configFilePath = ""

	config.SetQueryTracingEnabled(true)
	config.SetQueryTracingThreshold(time.Second)

	data, err := format_2_0.marshal(config)
	c.Assert(err, jc.ErrorIsNil)
	newConfig, err := format_2_0.unmarshal(data)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(newConfig, tc.DeepEquals, config)
	c.Check(newConfig.QueryTracingEnabled(), jc.IsTrue)
	c.Check(newConfig.QueryTracingThreshold(), tc.Equals, time.Second)
}

func (*format_2_0Suite) TestOpenTelemetry(c *tc.C) {
	config := newTestConfig(c)
	// configFilePath is not serialized as it is the location of the file.
	config.configFilePath = ""

	config.SetOpenTelemetryEnabled(true)
	config.SetOpenTelemetryEndpoint("http://foo.bar")
	config.SetOpenTelemetryInsecure(true)
	config.SetOpenTelemetryStackTraces(true)
	config.SetOpenTelemetrySampleRatio(0.5)
	config.SetOpenTelemetryTailSamplingThreshold(time.Second)

	data, err := format_2_0.marshal(config)
	c.Assert(err, jc.ErrorIsNil)
	newConfig, err := format_2_0.unmarshal(data)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(newConfig, tc.DeepEquals, config)
	c.Check(newConfig.OpenTelemetryEnabled(), jc.IsTrue)
	c.Check(newConfig.OpenTelemetryEndpoint(), tc.Equals, "http://foo.bar")
	c.Check(newConfig.OpenTelemetryInsecure(), jc.IsTrue)
	c.Check(newConfig.OpenTelemetryStackTraces(), jc.IsTrue)
	c.Check(newConfig.OpenTelemetrySampleRatio(), tc.Equals, 0.5)
	c.Check(newConfig.OpenTelemetryTailSamplingThreshold(), tc.Equals, time.Second)
}

func (*format_2_0Suite) TestObjectStore(c *tc.C) {
	config := newTestConfig(c)
	// configFilePath is not serialized as it is the location of the file.
	config.configFilePath = ""

	config.SetObjectStoreType(objectstore.FileBackend)

	data, err := format_2_0.marshal(config)
	c.Assert(err, jc.ErrorIsNil)
	newConfig, err := format_2_0.unmarshal(data)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(newConfig, tc.DeepEquals, config)
	c.Check(newConfig.ObjectStoreType(), tc.Equals, objectstore.FileBackend)
}

var agentConfig2_0Contents = `
# format 2.0
controller: controller-deadbeef-1bad-500d-9000-4b1d0d06f00d
model: model-deadbeef-0bad-400d-8000-4b1d0d06f00d
tag: machine-0
datadir: /home/user/.local/share/juju/local
logdir: /var/log/juju-user-local
nonce: user-admin:bootstrap
jobs:
- JobManageModel
upgradedToVersion: 1.17.5.1
cacert: '-----BEGIN CERTIFICATE-----

  MIICWzCCAcagAwIBAgIBADALBgkqhkiG9w0BAQUwQzENMAsGA1UEChMEanVqdTEy

  MDAGA1UEAwwpanVqdS1nZW5lcmF0ZWQgQ0EgZm9yIGVudmlyb25tZW50ICJsb2Nh

  bCIwHhcNMTQwMzA1MTQxOTA3WhcNMjQwMzA1MTQyNDA3WjBDMQ0wCwYDVQQKEwRq

  dWp1MTIwMAYDVQQDDClqdWp1LWdlbmVyYXRlZCBDQSBmb3IgZW52aXJvbm1lbnQg

  ImxvY2FsIjCBnzANBgkqhkiG9w0BAQEFAAOBjQAwgYkCgYEAwHsKV7fKfmSQt2QL

  P4+hrqQJhDTMifgNkIY9nTlLHegV5jl5XJ8lRYjZBXJEMz0AzW/RbrDElkn5+4Do

  pIWPNDAT0eztXBvVwL6qQOUtiBsA7vHQJMQaLVAmZNKvrHyuhcoG+hpf8EMaLdbA

  iCGKifs+Y0MFt5AeriVDH5lGlzcCAwEAAaNjMGEwDgYDVR0PAQH/BAQDAgCkMA8G

  A1UdEwEB/wQFMAMBAf8wHQYDVR0OBBYEFB3Td3SP66UToZkOjVh3Wy8b6HR6MB8G

  A1UdIwQYMBaAFB3Td3SP66UToZkOjVh3Wy8b6HR6MAsGCSqGSIb3DQEBBQOBgQB4

  izvSRSpimi40aEOnZIsSMHVBiSCclpBg5cq7lGyiUSsDROTIbsRAKPBmrflB/qbf

  J70rWFwh/d/5ssCAYrZviFL6WvpuLD3j3m4PYampNMmvJf2s6zVRIMotEY+bVwfU

  z4jGaVpODac0i0bE0/Uh9qXK1UXcYY57vNNAgkaYAQ==

  -----END CERTIFICATE-----

'
stateaddresses:
- localhost:37017
statepassword: NB5imrDaWCCRW/4akSSvUxhX
apiaddresses:
- localhost:17071
apipassword: NB5imrDaWCCRW/4akSSvUxhX
oldpassword: oBlMbFUGvCb2PMFgYVzjS6GD
values:
  AGENT_SERVICE_NAME: juju-agent-user-local
  CONTAINER_TYPE: ""
  NAMESPACE: user-local
  PROVIDER_TYPE: local
  STORAGE_ADDR: 10.0.3.1:8040
  STORAGE_DIR: /home/user/.local/share/juju/local/storage
controllercert: '-----BEGIN CERTIFICATE-----

  MIICNzCCAaKgAwIBAgIBADALBgkqhkiG9w0BAQUwQzENMAsGA1UEChMEanVqdTEy

  MDAGA1UEAwwpanVqdS1nZW5lcmF0ZWQgQ0EgZm9yIGVudmlyb25tZW50ICJsb2Nh

  bCIwHhcNMTQwMzA1MTQxOTE1WhcNMjQwMzA1MTQyNDE1WjAbMQ0wCwYDVQQKEwRq

  dWp1MQowCAYDVQQDEwEqMIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDJnbuN

  L3m/oY7Er2lEF6ye1SodepvpI0CLCdLwrYP52cRxbVzoD1jbXveclolg2xoUquga

  qxsAhvVzzGaoLux1BoBD+G0N637fnY4XSIC9IuSkPOAdReKJkOvTL4nTjpzgfeHR

  hRin6Xckvp96L4Prmki7sYQ8PG9Q7TBcOf4yowIDAQABo2cwZTAOBgNVHQ8BAf8E

  BAMCAKgwEwYDVR0lBAwwCgYIKwYBBQUHAwEwHQYDVR0OBBYEFE1MB3d+5BW+n066

  lWcVkhta1etlMB8GA1UdIwQYMBaAFB3Td3SP66UToZkOjVh3Wy8b6HR6MAsGCSqG

  SIb3DQEBBQOBgQBnsBvl3hfIQbHhAlqritDBCWGpaXywlHe4PvyVL3LZTLiAZ9a/

  BOSBfovs81sjUe5l60j+1vgRQgvT2Pnw6WGWmYWhSyxW7upEUl1LuZxnw3AVGVFO

  r140iBNUtTfGUf3PmyBXHSotqgMime+rNSjl25qSoYwnuQXdFdCKJoutYg==

  -----END CERTIFICATE-----

'
controllerkey: '-----BEGIN RSA PRIVATE KEY-----

  MIICXAIBAAKBgQDJnbuNL3m/oY7Er2lEF6ye1SodepvpI0CLCdLwrYP52cRxbVzo

  D1jbXveclolg2xoUqugaqxsAhvVzzGaoLux1BoBD+G0N637fnY4XSIC9IuSkPOAd

  ReKJkOvTL4nTjpzgfeHRhRin6Xckvp96L4Prmki7sYQ8PG9Q7TBcOf4yowIDAQAB

  AoGASEtzETFQ6tI3q3dqu6vxjhLJw0BP381wO2sOZJcTl+fqdPHOOrgmGKN5DoE8

  SarHM1oFWGq6h/nc0eUdenk4+CokpbKRgUU9hB1TKGYMbN3bUTKPOqTMHbnrhWdT

  P/fqa+nXhvg7igMT3Rk7l9DsSxoYB5xZmiLaXqynVE5MNoECQQDRsgDDUrUOeMH6

  1+GO+afb8beRzR8mnaBvja6XLlZB6SUcGet9bMgAiGH3arH6ARfNNsWrDAmvArah

  SKeqRB5TAkEA9iMEQDkcybCmxu4Y3YLeQuT9r3h26QhQjc+eRINS/3ZLN+lxKnXG

  N019ZUlsyL97lJBDzTMPsBqfXJ2pbqXwcQJBAJNLuPN63kl7E68zA3Ld9UYvBWY6

  Mp56bJ7PZAs39kk4DuQtZNhmmBqfskkMPlZBfEmfJrxeqVKw0j56faPBU5cCQFYU

  mP/8+VxwM2OPEZMmmaS7gR1E/BEznzh5S9iaNQSy0kuTkMhQuCnPJ/OsYiczEH08

  lvnEyc/E/8bcPM09q4ECQCFwMWzw2Jx9VOBGm60yiOKIdLgdDZOY/tP0jigNCMJF

  47/BJx3FCgW3io81a4KOc445LxgiPUJyyCNlY1dW70o=

  -----END RSA PRIVATE KEY-----

'
caprivatekey: '-----BEGIN RSA PRIVATE KEY-----

  MIICXAIBAAKBgQDJnbuNL3m/oY7Er2lEF6ye1SodepvpI0CLCdLwrYP52cRxbVzo

  D1jbXveclolg2xoUqugaqxsAhvVzzGaoLux1BoBD+G0N637fnY4XSIC9IuSkPOAd

  ReKJkOvTL4nTjpzgfeHRhRin6Xckvp96L4Prmki7sYQ8PG9Q7TBcOf4yowIDAQAB

  AoGASEtzETFQ6tI3q3dqu6vxjhLJw0BP381wO2sOZJcTl+fqdPHOOrgmGKN5DoE8

  SarHM1oFWGq6h/nc0eUdenk4+CokpbKRgUU9hB1TKGYMbN3bUTKPOqTMHbnrhWdT

  P/fqa+nXhvg7igMT3Rk7l9DsSxoYB5xZmiLaXqynVE5MNoECQQDRsgDDUrUOeMH6

  1+GO+afb8beRzR8mnaBvja6XLlZB6SUcGet9bMgAiGH3arH6ARfNNsWrDAmvArah

  SKeqRB5TAkEA9iMEQDkcybCmxu4Y3YLeQuT9r3h26QhQjc+eRINS/3ZLN+lxKnXG

  N019ZUlsyL97lJBDzTMPsBqfXJ2pbqXwcQJBAJNLuPN63kl7E68zA3Ld9UYvBWY6

  Mp56bJ7PZAs39kk4DuQtZNhmmBqfskkMPlZBfEmfJrxeqVKw0j56faPBU5cCQFYU

  mP/8+VxwM2OPEZMmmaS7gR1E/BEznzh5S9iaNQSy0kuTkMhQuCnPJ/OsYiczEH08

  lvnEyc/E/8bcPM09q4ECQCFwMWzw2Jx9VOBGm60yiOKIdLgdDZOY/tP0jigNCMJF

  47/BJx3FCgW3io81a4KOc445LxgiPUJyyCNlY1dW70o=

  -----END RSA PRIVATE KEY-----

'
apiport: 17070
controllerapiport: 17071
`[1:]

var agentConfig2_0NotStateMachine = `
# format 2.0
controller: controller-deadbeef-1bad-500d-9000-4b1d0d06f00d
model: model-deadbeef-0bad-400d-8000-4b1d0d06f00d
tag: machine-1
datadir: /home/user/.local/share/juju/local
logdir: /var/log/juju-user-local
nonce: user-admin:bootstrap
jobs:
- JobManageModel
upgradedToVersion: 1.17.5.1
cacert: '-----BEGIN CERTIFICATE-----

  MIICWzCCAcagAwIBAgIBADALBgkqhkiG9w0BAQUwQzENMAsGA1UEChMEanVqdTEy

  MDAGA1UEAwwpanVqdS1nZW5lcmF0ZWQgQ0EgZm9yIGVudmlyb25tZW50ICJsb2Nh

  bCIwHhcNMTQwMzA1MTQxOTA3WhcNMjQwMzA1MTQyNDA3WjBDMQ0wCwYDVQQKEwRq

  dWp1MTIwMAYDVQQDDClqdWp1LWdlbmVyYXRlZCBDQSBmb3IgZW52aXJvbm1lbnQg

  ImxvY2FsIjCBnzANBgkqhkiG9w0BAQEFAAOBjQAwgYkCgYEAwHsKV7fKfmSQt2QL

  P4+hrqQJhDTMifgNkIY9nTlLHegV5jl5XJ8lRYjZBXJEMz0AzW/RbrDElkn5+4Do

  pIWPNDAT0eztXBvVwL6qQOUtiBsA7vHQJMQaLVAmZNKvrHyuhcoG+hpf8EMaLdbA

  iCGKifs+Y0MFt5AeriVDH5lGlzcCAwEAAaNjMGEwDgYDVR0PAQH/BAQDAgCkMA8G

  A1UdEwEB/wQFMAMBAf8wHQYDVR0OBBYEFB3Td3SP66UToZkOjVh3Wy8b6HR6MB8G

  A1UdIwQYMBaAFB3Td3SP66UToZkOjVh3Wy8b6HR6MAsGCSqGSIb3DQEBBQOBgQB4

  izvSRSpimi40aEOnZIsSMHVBiSCclpBg5cq7lGyiUSsDROTIbsRAKPBmrflB/qbf

  J70rWFwh/d/5ssCAYrZviFL6WvpuLD3j3m4PYampNMmvJf2s6zVRIMotEY+bVwfU

  z4jGaVpODac0i0bE0/Uh9qXK1UXcYY57vNNAgkaYAQ==

  -----END CERTIFICATE-----

'
stateaddresses:
- localhost:37017
statepassword: NB5imrDaWCCRW/4akSSvUxhX
apiaddresses:
- localhost:17070
apipassword: NB5imrDaWCCRW/4akSSvUxhX
oldpassword: oBlMbFUGvCb2PMFgYVzjS6GD
values:
  AGENT_SERVICE_NAME: juju-agent-user-local
  CONTAINER_TYPE: ""
  MONGO_SERVICE_NAME: juju-db-user-local
  NAMESPACE: user-local
  PROVIDER_TYPE: local
  STORAGE_ADDR: 10.0.3.1:8040
  STORAGE_DIR: /home/user/.local/share/juju/local/storage
apiport: 17070
`[1:]
