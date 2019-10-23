// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"bytes"
	"io"

	"github.com/juju/errors"
	utilexec "github.com/juju/utils/exec"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner"
)

//go:generate mockgen -package mocks -destination mocks/exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor
//go:generate mockgen -package mocks -destination mocks/uniter_mock.go github.com/juju/juju/worker/uniter ProviderIDGetter
func getNewRunnerExecutor(
	execClient exec.Executor,
	operatorPaths Paths,
	operatorInfo caas.OperatorInfo,
) uniter.NewRunnerExecutorFunc {
	return func(providerIDGetter uniter.ProviderIDGetter, unitPaths uniter.Paths) runner.ExecFunc {
		return func(params runner.ExecParams) (*utilexec.ExecResponse, error) {
			if err := providerIDGetter.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
			podNameOrID := providerIDGetter.ProviderID()
			unitName := providerIDGetter.Name()
			logger.Debugf("exec on pod %q for unit %q, cmd %v", podNameOrID, unitName, params.Commands)
			if podNameOrID == "" {
				return nil, errors.NotFoundf("pod for %q", unitName)
			}

			// juju run - return stdout and stderr to ExecResponse.
			exitErr := execClient.Exec(
				exec.ExecParams{
					PodName:    podNameOrID,
					Commands:   params.Commands,
					WorkingDir: params.WorkingDir,
					Env:        params.Env,
					Stdout:     params.Stdout,
					Stderr:     params.Stderr,
				},
				params.Cancel,
			)
			exitErr = errors.Cause(exitErr)
			if params.StdoutLogger != nil {
				params.StdoutLogger.Stop()
			}
			if params.StderrLogger != nil {
				params.StderrLogger.Stop()
			}

			readBytes := func(r io.Reader) []byte {
				var o bytes.Buffer
				o.ReadFrom(r)
				return o.Bytes()
			}
			exitCode := func(exitErr error) int {
				if exitErr != nil {
					if exitErr, ok := exitErr.(exec.ExitError); ok {
						return exitErr.ExitStatus()
					}
					return -1
				}
				return 0
			}
			return &utilexec.ExecResponse{
				Code:   exitCode(exitErr),
				Stdout: readBytes(params.Stdout),
				Stderr: readBytes(params.Stderr),
			}, exitErr
		}
	}
}
