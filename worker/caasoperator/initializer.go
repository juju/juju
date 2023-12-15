// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/caas"
	caasconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/cmd/jujud/agent/config"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/wrench"
)

// initializeUnitParams contains parameters and dependencies for initializing
// a unit.
type initializeUnitParams struct {
	// UnitTag of the unit being initialized.
	UnitTag names.UnitTag

	// ProviderID is the pod-name or pod-uid
	ProviderID string

	// Logger for the worker.
	Logger Logger

	// Paths provides CAAS operator paths.
	Paths Paths

	// OperatorInfo contains serving information such as Certs and PrivateKeys.
	OperatorInfo caas.OperatorInfo

	// ExecClient is used for initializing units.
	ExecClient exec.Executor

	// WriteFile is used to write files to the local state.
	WriteFile func(string, []byte, os.FileMode) error

	// TempDir is used for creating a temporary directory.
	TempDir func(string, string) (string, error)

	// Clock holds the clock to be used by the runner.
	Clock clock.Clock

	// reTrier is used for re-running some certain retryable exec request.
	ReTrier reTrier
}

// Validate initializeUnitParams
func (p initializeUnitParams) Validate() error {
	if p.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if p.ProviderID == "" {
		return errors.NotValidf("missing ProviderID")
	}
	if p.ExecClient == nil {
		return errors.NotValidf("missing ExecClient")
	}
	if p.WriteFile == nil {
		return errors.NotValidf("missing WriteFile")
	}
	if p.TempDir == nil {
		return errors.NotValidf("missing TempDir")
	}
	return nil
}

// reTrier is used for re-running some certain retryable exec request.
type reTrier func(func() error, func(error) bool, Logger, clock.Clock, <-chan struct{}) error

// runnerWithRetry retries the exec request for init unit process if it got a retryable error.
func runnerWithRetry(f func() error, fatalChecker func(error) bool, logger Logger, clk clock.Clock, cancel <-chan struct{}) error {
	do := func() error {
		if wrench.IsActive("exec", "retryable-error") {
			fakeErr := errors.New("fake retryable-error")
			logger.Warningf("wrench exec retryable-error enabled, returns %v", fakeErr)
			return exec.NewExecRetryableError(fakeErr)
		}
		return f()
	}
	args := retry.CallArgs{
		Attempts:     5,
		Delay:        2 * time.Second,
		MaxDuration:  30 * time.Second,
		Clock:        clk,
		Stop:         cancel,
		Func:         do,
		IsFatalError: fatalChecker,
		NotifyFunc: func(err error, attempt int) {
			logger.Debugf("retrying exec request, in %d attempt, %v", attempt, err)
		},
	}
	return errors.Trace(retry.Call(args))
}

// initializeUnit with the charm and configuration.
func initializeUnit(params initializeUnitParams, cancel <-chan struct{}) error {
	if err := params.Validate(); err != nil {
		return errors.Trace(err)
	}

	params.Logger.Infof("started pod init on %q", params.UnitTag.Id())
	container := caas.InitContainerName
	initArgs := []string{"--unit", params.UnitTag.String()}

	rootToolsDir := tools.ToolsDir(config.DataDir, "")
	jujudPath := filepath.Join(rootToolsDir, "jujud")
	unitPaths := uniter.NewPaths(config.DataDir, params.UnitTag, nil)
	operatorPaths := params.Paths
	tempDir, err := params.TempDir(os.TempDir(), params.UnitTag.String())
	if err != nil {
		return errors.Annotatef(err, "creating temp directory")
	}

	stdout := &bytes.Buffer{}
	command := []string{"mkdir", "-p", tempDir}
	err = params.ExecClient.Exec(exec.ExecParams{
		Commands:      command,
		PodName:       params.ProviderID,
		ContainerName: container,
		Stdout:        stdout,
		Stderr:        stdout,
	}, cancel)
	if err != nil {
		return errors.Annotatef(err, "running command: %q failed: %q", strings.Join(command, " "), string(stdout.Bytes()))
	}

	tempCharmDir := filepath.Join(tempDir, "charm")
	// This heavy exec task might get 137 error, we will retry if it does happen.
	err = params.ReTrier(
		func() error {
			return params.ExecClient.Copy(exec.CopyParams{
				Src: exec.FileResource{
					Path: operatorPaths.State.CharmDir,
				},
				Dest: exec.FileResource{
					Path:          tempDir,
					PodName:       params.ProviderID,
					ContainerName: container,
				},
			}, cancel)
		},
		func(err error) bool {
			return err != nil && !exec.IsExecRetryableError(err)
		}, params.Logger, params.Clock, cancel,
	)
	if err != nil {
		return errors.Trace(err)
	}
	tempOperatorCacheFile, tempCACertFile, err := setupRemoteConfiguration(params, cancel, unitPaths, tempDir, container)
	if err != nil {
		return errors.Trace(err)
	}
	initArgs = append(initArgs,
		"--charm-dir", tempCharmDir,
		"--send", // Init container will wait for us to send the data.
		"--operator-file", tempOperatorCacheFile,
		"--operator-ca-cert-file", tempCACertFile,
	)

	stdout = &bytes.Buffer{}
	command = append([]string{jujudPath, "caas-unit-init"}, initArgs...)
	err = params.ExecClient.Exec(exec.ExecParams{
		Commands:      command,
		PodName:       params.ProviderID,
		ContainerName: container,
		WorkingDir:    config.DataDir,
		Stdout:        stdout,
		Stderr:        stdout,
	}, cancel)
	if err != nil {
		return errors.Annotatef(err, "caas-unit-init for unit %q with command: %q failed: %s", params.UnitTag.Id(), strings.Join(command, " "), string(stdout.Bytes()))
	}
	return nil
}

func setupRemoteConfiguration(params initializeUnitParams, cancel <-chan struct{},
	unitPaths uniter.Paths, tempDir string, container string) (string, string, error) {
	tempCACertFile := filepath.Join(tempDir, caas.CACertFile)
	if err := params.WriteFile(tempCACertFile, []byte(params.OperatorInfo.CACert), 0644); err != nil {
		return "", "", errors.Trace(err)
	}
	err := params.ExecClient.Copy(exec.CopyParams{
		Src: exec.FileResource{
			Path: tempCACertFile,
		},
		Dest: exec.FileResource{
			Path:          tempCACertFile,
			PodName:       params.ProviderID,
			ContainerName: container,
		},
	}, cancel)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	serviceAddress := os.Getenv(caasconstants.OperatorServiceIPEnvName)
	params.Logger.Debugf("operator service address: %v", serviceAddress)
	token, err := utils.RandomPassword()
	if err != nil {
		return "", "", errors.Trace(err)
	}
	clientInfo := caas.OperatorClientInfo{
		ServiceAddress: serviceAddress,
		Token:          token,
	}
	data, err := clientInfo.Marshal()
	if err != nil {
		return "", "", errors.Trace(err)
	}
	operatorCacheFile := filepath.Join(unitPaths.State.BaseDir, caas.OperatorClientInfoCacheFile)
	if err := params.WriteFile(operatorCacheFile, data, 0644); err != nil {
		return "", "", errors.Trace(err)
	}
	tempOperatorCacheFile := filepath.Join(tempDir, caas.OperatorClientInfoCacheFile)
	err = params.ExecClient.Copy(exec.CopyParams{
		Src: exec.FileResource{
			Path: operatorCacheFile,
		},
		Dest: exec.FileResource{
			Path:          tempOperatorCacheFile,
			PodName:       params.ProviderID,
			ContainerName: container,
		},
	}, cancel)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	return tempOperatorCacheFile, tempCACertFile, nil
}
