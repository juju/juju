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
	"github.com/juju/utils/exec"
)

type Bridger interface {
	Bridge(deviceNames []string) error
}

type ScriptResult struct {
	Stdout   []byte
	Stderr   []byte
	Code     int
	TimedOut bool
}

type etcNetworkInterfacesBridger struct {
	Clock        clock.Clock
	Timeout      time.Duration
	BridgePrefix string
	Filename     string
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
	prefix := ""
	if b.BridgePrefix != "" {
		prefix = fmt.Sprintf("--bridge-prefix=%s", b.BridgePrefix)
	}
	cmd := fmt.Sprintf(`%s - --interfaces-to-bridge=%q --activate %s %s <<'EOF'
%s
EOF
`,
		bestPythonVersion(),
		strings.Join(deviceNames, " "),
		prefix,
		b.Filename,
		BridgeScriptPythonContent)

	result, err := RunCommand(cmd, os.Environ(), b.Clock, b.Timeout)
	logger.Infof("bridgescript command=%s", cmd)
	logger.Infof("bridgescript result=%v, timeout=%v", result.Code, result.TimedOut)
	if result.Code != 0 {
		logger.Errorf("bridgescript stdout\n%s\n", result.Stdout)
		logger.Errorf("bridgescript stderr\n%s\n", result.Stderr)
	}
	if result.TimedOut {
		return errors.Errorf("bridgescript timed out after %v", b.Timeout)
	}
	return err
}

func NewEtcNetworkInterfacesBridger(clock clock.Clock, timeout time.Duration, bridgePrefix, filename string) Bridger {
	return &etcNetworkInterfacesBridger{
		Clock:        clock,
		Timeout:      timeout,
		BridgePrefix: bridgePrefix,
		Filename:     filename,
	}
}

func RunCommand(command string, environ []string, clock clock.Clock, timeout time.Duration) (*ScriptResult, error) {
	cmd := exec.RunParams{
		Commands:    command,
		Environment: environ,
		Clock:       clock,
	}

	err := cmd.Run()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cancel chan struct{}
	timedOut := false

	if timeout != 0 {
		cancel = make(chan struct{})
		go func() {
			<-clock.After(timeout)
			timedOut = true
			close(cancel)
		}()
	}

	result, err := cmd.WaitWithCancel(cancel)

	return &ScriptResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		Code:     result.Code,
		TimedOut: timedOut,
	}, nil
}
