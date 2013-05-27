// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

func NewTestSimpleContext(deployerTag, initDir, dataDir, logDir, syslogConfigDir string) *SimpleContext {
	return &SimpleContext{
		caCert:          []byte("test-cert"),
		deployerTag:     deployerTag,
		initDir:         initDir,
		dataDir:         dataDir,
		logDir:          logDir,
		syslogConfigDir: syslogConfigDir,
		stateAddrs:      []string{"s1:123", "s2:123"},
		apiAddrs:        []string{"a1:123", "a2:123"},
	}
}
