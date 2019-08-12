// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"bytes"
	"io"
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
			Commands: []string{"mkdir", "-p", path},
			Stdout:   stdout,
			Stderr:   stderr,
		},
		cancel,
	)
	return errors.Trace(err)
}

func syncFiles(
	client exec.Executor,
	podName string,
	cancel <-chan struct{},
	filesDirs []string,
) error {
	for _, path := range filesDirs {
		logger.Debugf("syncing files at %q", path)
		if err := client.Copy(exec.CopyParam{
			Src: exec.FileResource{
				Path: path,
			},
			Dest: exec.FileResource{
				Path:    path,
				PodName: podName,
			},
		}, cancel); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func prepare(
	client exec.Executor,
	podName string,
	dataDir string,
	stdout io.Writer,
	stderr io.Writer,
	cancel <-chan struct{},
) error {
	// ensuring data dir.
	if err := ensurePath(client, podName, dataDir, stdout, stderr, cancel); err != nil {
		return errors.Trace(err)
	}

	// syncing files.
	// TODO(caas): add a new cmd for checking jujud version, charm/version etc.
	// exec run this new cmd to decide if we need re-push files or not.
	err := syncFiles(
		client, podName, cancel,
		[]string{
			// TODO(caas): only sync files required for actions/run.
			filepath.Join(dataDir, "agents"),
			filepath.Join(dataDir, "tools"),
		},
	)
	return errors.Trace(err)
}

//go:generate mockgen -package mocks -destination mocks/exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor
//go:generate mockgen -package mocks -destination mocks/uniter_mock.go github.com/juju/juju/worker/uniter ProviderIDGetter
func getNewRunnerExecutor(
	execClient exec.Executor,
	dataDir string,
) uniter.NewRunnerExecutorFunc {
	return func(providerIDGetter uniter.ProviderIDGetter) runner.ExecFunc {
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

			if err := prepare(execClient, podNameOrID, dataDir, params.Stdout, params.Stderr, params.Cancel); err != nil {
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
