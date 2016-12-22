// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/exec"
)

type BridgerConfig struct {
	Clock        clock.Clock
	Timeout      time.Duration
	BridgePrefix string
	InputFile    string
}

type Bridger interface {
	Bridge(deviceNames []string) error
}

type etcNetworkInterfacesBridger struct {
	cfg BridgerConfig
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

func writePythonScript(content string, perms os.FileMode) (string, error) {
	tmpfile, err := ioutil.TempFile("", "add-bridge")

	if err != nil {
		return "", errors.Trace(err)
	}

	if err := tmpfile.Close(); err != nil {
		os.Remove(tmpfile.Name())
		return "", errors.Trace(err)
	}

	if err := ioutil.WriteFile(tmpfile.Name(), []byte(content), perms); err != nil {
		os.Remove(tmpfile.Name())
		return "", errors.Trace(err)
	}

	if err := os.Chmod(tmpfile.Name(), 0755); err != nil {
		os.Remove(tmpfile.Name())
		return "", errors.Trace(err)
	}

	return tmpfile.Name(), nil
}

func runCommandWithTimeout(command string, timeout time.Duration, clock clock.Clock) (*exec.ExecResponse, error) {
	cmd := exec.RunParams{
		Commands:    command,
		Environment: os.Environ(),
		Clock:       clock,
	}

	err := cmd.Run()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cancel chan struct{}
	if timeout != 0 {
		cancel = make(chan struct{})
		go func() {
			<-clock.After(timeout)
			close(cancel)
		}()
	}

	return cmd.WaitWithCancel(cancel)
}

func (b *etcNetworkInterfacesBridger) Bridge(deviceNames []string) error {
	pythonVersion := bestPythonVersion()
	if pythonVersion == "" {
		return errors.New("no Python installation found")
	}

	content := fmt.Sprintf("#!%s\n%s\n", pythonVersion, BridgeScriptPythonContent)
	tmpfile, err := writePythonScript(content, 0755)

	if err != nil {
		return errors.Annotatef(err, "failed to write bridgescript")
	}

	defer os.Remove(tmpfile)

	command := fmt.Sprintf("%s %q --activate --bridge-prefix=%q --interfaces-to-bridge=%q %q",
		pythonVersion, tmpfile, b.cfg.BridgePrefix, strings.Join(deviceNames, " "), b.cfg.InputFile)

	logger.Infof("running %q with timeout=%v", command, b.cfg.Timeout)

	result, err := runCommandWithTimeout(command, time.Duration(b.cfg.Timeout), b.cfg.Clock)

	if err != nil {
		return errors.Trace(err)
	}

	logger.Infof("bridgescript result=%v", result.Code)

	if result.Code != 0 {
		logger.Errorf("bridgescript stdout\n%s\n", result.Stdout)
		logger.Errorf("bridgescript stderr\n%s\n", result.Stderr)
	}

	return err
}

func NewEtcNetworkInterfacesBridger(clock clock.Clock, timeout time.Duration, bridgePrefix, filename string) Bridger {
	return &etcNetworkInterfacesBridger{
		cfg: BridgerConfig{
			Clock:        clock,
			Timeout:      timeout,
			BridgePrefix: bridgePrefix,
			InputFile:    filename,
		},
	}
}
