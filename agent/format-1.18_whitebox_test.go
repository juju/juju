// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The format tests are white box tests, meaning that the tests are in the
// same package as the code, as all the format details are internal to the
// package.

package agent

import (
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

type format_1_18Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&format_1_18Suite{})

var configData1_18WithoutUpgradedToVersion = "# format 1.18\n" + configDataWithoutNewAttributes

func (s *format_1_18Suite) TestMissingAttributes(c *gc.C) {
	dataDir := c.MkDir()
	configPath := filepath.Join(dataDir, agentConfigFilename)
	err := utils.AtomicWriteFile(configPath, []byte(configData1_18WithoutUpgradedToVersion), 0600)
	c.Assert(err, gc.IsNil)
	readConfig, err := ReadConfig(configPath)
	c.Assert(err, gc.IsNil)
	c.Assert(readConfig.UpgradedToVersion(), gc.Equals, version.MustParse("1.16.0"))
	c.Assert(readConfig.LogDir(), gc.Equals, "/var/log/juju")
	c.Assert(readConfig.DataDir(), gc.Equals, "/var/lib/juju")
}

func (s *format_1_18Suite) TestStatePortNotParsedWithoutSecret(c *gc.C) {
	dataDir := c.MkDir()
	configPath := filepath.Join(dataDir, agentConfigFilename)
	err := utils.AtomicWriteFile(configPath, []byte(agentConfig1_18NotStateMachine), 0600)
	c.Assert(err, gc.IsNil)
	readConfig, err := ReadConfig(configPath)
	c.Assert(err, gc.IsNil)
	_, available := readConfig.StateServingInfo()
	c.Assert(available, gc.Equals, false)
}

func (*format_1_18Suite) TestReadConfWithExisting1_18ConfigFileContents(c *gc.C) {
	dataDir := c.MkDir()
	configPath := filepath.Join(dataDir, agentConfigFilename)
	err := utils.AtomicWriteFile(configPath, []byte(agentConfig1_18Contents), 0600)
	c.Assert(err, gc.IsNil)

	config, err := ReadConfig(configPath)
	c.Assert(err, gc.IsNil)
	c.Assert(config.UpgradedToVersion(), jc.DeepEquals, version.MustParse("1.17.5.1"))
	c.Assert(config.Jobs(), jc.DeepEquals, []params.MachineJob{params.JobManageEnviron})
}

var agentConfig1_18Contents = `
# format 1.18
tag: machine-0
datadir: /home/user/.juju/local
logdir: /var/log/juju-user-local
nonce: user-admin:bootstrap
jobs:
- JobManageEnviron
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
  NAMESPACE: user-local
  PROVIDER_TYPE: local
  STORAGE_ADDR: 10.0.3.1:8040
  STORAGE_DIR: /home/user/.juju/local/storage
stateservercert: '-----BEGIN CERTIFICATE-----

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
stateserverkey: '-----BEGIN RSA PRIVATE KEY-----

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
`[1:]

var agentConfig1_18NotStateMachine = `
# format 1.18
tag: machine-1
datadir: /home/user/.juju/local
logdir: /var/log/juju-user-local
nonce: user-admin:bootstrap
jobs:
- JobManageEnviron
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
  STORAGE_DIR: /home/user/.juju/local/storage
apiport: 17070
`[1:]
