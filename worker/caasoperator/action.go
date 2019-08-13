// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	utilexec "github.com/juju/utils/exec"

	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner"
)

func ensurePath(
	client exec.Executor,
	podName string,
	path string,
	stdout io.Writer,
	stderr io.Writer,
	cancel <-chan struct{},
) error {
	logger.Debugf("ensuring %q", path)
	err := client.Exec(
		exec.ExecParams{
			PodName:  podName,
			Commands: []string{"test", "-d", path, "||", "mkdir", "-p", path},
			Stdout:   stdout,
			Stderr:   stderr,
		},
		cancel,
	)
	return errors.Trace(err)
}

func prepare(
	client exec.Executor,
	podName string,
	operatorPaths Paths,
	unitPaths uniter.Paths,
	stdout io.Writer,
	stderr io.Writer,
	cancel <-chan struct{},
) error {
	for _, path := range []string{
		// order matters here.
		operatorPaths.GetCharmDir(),
		unitPaths.GetCharmDir(),

		filepath.Join(operatorPaths.GetToolsDir(), "jujud"),
		unitPaths.GetToolsDir(),
	} {

		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			return errors.NotFoundf("file or path %q", path)
		}
		if err != nil {
			return errors.Trace(err)
		}
		destPath := filepath.Dir(path)
		if err := ensurePath(client, podName, destPath, stdout, stderr, cancel); err != nil {
			return errors.Trace(err)
		}

		if err := client.Copy(exec.CopyParam{
			Src: exec.FileResource{
				Path: path,
			},
			Dest: exec.FileResource{
				Path:    destPath,
				PodName: podName,
			},
		}, cancel); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

//go:generate mockgen -package mocks -destination mocks/exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor
//go:generate mockgen -package mocks -destination mocks/uniter_mock.go github.com/juju/juju/worker/uniter ProviderIDGetter
func getNewRunnerExecutor(
	execClient exec.Executor,
	operatorPaths Paths,
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

			if err := prepare(
				execClient, podNameOrID,
				operatorPaths, unitPaths,
				params.Stdout, params.Stderr, params.Cancel,
			); err != nil {
				logger.Errorf("ensuring dirs and syncing files %v", err)
				return nil, errors.Trace(err)
			}

			// juju run - return stdout and stderr to ExecResponse.
			if err := execClient.Exec(
				exec.ExecParams{
					PodName:    podNameOrID,
					Commands:   params.Commands,
					WorkingDir: params.WorkingDir,
					Env:        params.Env,
					Stdout:     params.Stdout,
					Stderr:     params.Stderr,
				},
				params.Cancel,
			); err != nil {
				return nil, errors.Trace(err)
			}

			readBytes := func(r io.Reader) []byte {
				var o bytes.Buffer
				o.ReadFrom(r)
				return o.Bytes()
			}
			if params.ProcessResponse {
				return &utilexec.ExecResponse{
					Stdout: readBytes(params.Stdout),
					Stderr: readBytes(params.Stderr),
				}, nil
			}
			return &utilexec.ExecResponse{}, nil
		}
	}
}
