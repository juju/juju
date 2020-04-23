// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"bytes"
	"io"

	"github.com/juju/errors"
	utilexec "github.com/juju/utils/exec"

	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner"
)

//go:generate mockgen -package mocks -destination mocks/exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor
//go:generate mockgen -package mocks -destination mocks/uniter_mock.go github.com/juju/juju/worker/uniter ProviderIDGetter
func getNewRunnerExecutor(logger Logger, execClient exec.Executor) uniter.NewRunnerExecutorFunc {
	return func(providerIDGetter uniter.ProviderIDGetter, unitPaths uniter.Paths) runner.ExecFunc {
		return func(params runner.ExecParams) (*utilexec.ExecResponse, error) {
			return remoteExecute(logger, execClient, providerIDGetter, unitPaths, params)
		}
	}
}

func remoteExecute(logger Logger,
	execClient exec.Executor,
	providerIDGetter uniter.ProviderIDGetter,
	unitPaths uniter.Paths,
	params runner.ExecParams) (*utilexec.ExecResponse, error) {
	if err := providerIDGetter.Refresh(); err != nil {
		return nil, errors.Trace(err)
	}
	providerID := providerIDGetter.ProviderID()
	unitName := providerIDGetter.Name()
	logger.Debugf("exec on pod %q for unit %q, cmd %+q", providerID, unitName, params.Commands)
	if providerID == "" {
		return nil, errors.NotFoundf("pod for %q", unitName)
	}

	// juju run - return stdout and stderr to ExecResponse.
	err := execClient.Exec(
		exec.ExecParams{
			PodName:    providerID,
			Commands:   params.Commands,
			WorkingDir: params.WorkingDir,
			Env:        params.Env,
			Stdout:     params.Stdout,
			Stderr:     params.Stderr,
		},
		params.Cancel,
	)
	if params.StdoutLogger != nil {
		params.StdoutLogger.Stop()
	}
	if params.StderrLogger != nil {
		params.StderrLogger.Stop()
	}

	switch {
	case errors.IsNotFound(err):
		return nil, errors.Trace(err)
	case exec.IsContainerNotRunningError(err):
		return nil, errors.Trace(err)
	}

	readBytes := func(r io.Reader) []byte {
		var o bytes.Buffer
		o.ReadFrom(r)
		return o.Bytes()
	}
	exitCode := func(exitErr error) int {
		exitErr = errors.Cause(exitErr)
		if exitErr != nil {
			if exitErr, ok := exitErr.(exec.ExitError); ok {
				return exitErr.ExitStatus()
			}
			return -1
		}
		return 0
	}
	return &utilexec.ExecResponse{
		Code:   exitCode(err),
		Stdout: readBytes(params.Stdout),
		Stderr: readBytes(params.Stderr),
	}, err
}
