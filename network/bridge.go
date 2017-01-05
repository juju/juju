// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
)

// Bridger creates network bridges to support addressable containers.
type Bridger interface {
	// Turns existing devices into bridged devices.
	Bridge(deviceNames []string) error
}

type etcNetworkInterfacesBridger struct {
	BridgePrefix string
	Clock        clock.Clock
	DryRun       bool
	Environ      []string
	Filename     string
	Timeout      time.Duration
}

var _ Bridger = (*etcNetworkInterfacesBridger)(nil)

// bestPythonVersion returns a string to the best python interpreter
// found.
//
// For ubuntu series < xenial we prefer python2 over python3 as we
// don't want to invalidate lots of testing against known cloud-image
// contents. A summary of Ubuntu releases and python inclusion in the
// default install of Ubuntu Server is as follows:
//
// 12.04 precise:  python 2 (2.7.3)
// 14.04 trusty:   python 2 (2.7.5) and python3 (3.4.0)
// 14.10 utopic:   python 2 (2.7.8) and python3 (3.4.2)
// 15.04 vivid:    python 2 (2.7.9) and python3 (3.4.3)
// 15.10 wily:     python 2 (2.7.9) and python3 (3.4.3)
// 16.04 xenial:   python 3 only (3.5.1)
//
// going forward:  python 3 only
func bestPythonVersion() string {
	for _, version := range []string{
		"/usr/bin/python2",
		"/usr/bin/python3",
		"/usr/bin/python",
	} {
		if _, err := os.Stat(version); err == nil {
			return version
		}
	}
	return ""
}

func (b *etcNetworkInterfacesBridger) Bridge(deviceNames []string) error {
	cmd := bridgeCmd(deviceNames, b.BridgePrefix, b.Filename, BridgeScriptPythonContent, b.DryRun)
	logger.Debugf("bridgescript command=%s", cmd)
	result, err := runCommand(cmd, b.Environ, b.Clock, b.Timeout)
	if err != nil {
		return errors.Errorf("script invocation error: %s", err)
	}
	logger.Infof("bridgescript result=%v, timeout=%v", result.Code, result.TimedOut)
	if result.TimedOut {
		return errors.Errorf("bridgescript timed out after %v", b.Timeout)
	}
	if result.Code != 0 {
		logger.Errorf("bridgescript stdout\n%s\n", result.Stdout)
		logger.Errorf("bridgescript stderr\n%s\n", result.Stderr)
		return errors.Errorf("bridgescript failed: %s", string(result.Stderr))
	}
	return nil
}

func bridgeCmd(deviceNames []string, bridgePrefix, filename, pythonScript string, dryRun bool) string {
	dryRunOption := ""

	if bridgePrefix != "" {
		bridgePrefix = fmt.Sprintf("--bridge-prefix=%s", bridgePrefix)
	}

	if dryRun {
		dryRunOption = "--dry-run"
	}

	return fmt.Sprintf(`
%s - --interfaces-to-bridge=%q --activate %s %s %s <<'EOF'
%s
EOF
`[1:],
		bestPythonVersion(),
		strings.Join(deviceNames, " "),
		bridgePrefix,
		dryRunOption,
		filename,
		pythonScript)
}

// NewEtcNetworkInterfacesBridger returns a Bridger that can parse
// /etc/network/interfaces and create new stanzas to bridge existing
// interfaces.
//
// TODO(frobware): We shouldn't expose DryRun; once we implement the
// Python-based bridge script in Go this interface can change.
func NewEtcNetworkInterfacesBridger(environ []string, clock clock.Clock, timeout time.Duration, bridgePrefix, filename string, dryRun bool) Bridger {
	return &etcNetworkInterfacesBridger{
		BridgePrefix: bridgePrefix,
		Clock:        clock,
		DryRun:       dryRun,
		Environ:      environ,
		Filename:     filename,
		Timeout:      timeout,
	}
}
