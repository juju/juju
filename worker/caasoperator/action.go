// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"bytes"
	"path/filepath"

	"github.com/juju/errors"
	utilexec "github.com/juju/utils/exec"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner"
)

func ensurePath(
	client exec.Executor,
	podName string,
	cancel <-chan struct{},
	path string,
) error {
	var out bytes.Buffer
	err := client.Exec(
		exec.ExecParams{
			PodName:  podName,
			Commands: []string{"mkdir", "-p", path},
			Stdout:   &out,
			Stderr:   &out,
		},
		cancel,
	)
	logger.Debugf("ensuring %q, %q", path, out.String())
	return errors.Trace(err)
}

type symlink struct {
	Link   string
	Target string
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
	cancel <-chan struct{},
) error {
	// ensuring data dir.
	if err := ensurePath(client, podName, cancel, dataDir); err != nil {
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

func fetchPodNameForUnit(c UnitGetter, tag names.UnitTag) (string, error) {
	result, err := c.UnitsStatus(tag.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(result.Results) == 0 {
		return "", errors.NotFoundf("unit %q", tag.Id())
	}
	unit := result.Results[0]
	if unit.Error != nil {
		return "", unit.Error
	}
	return unit.Result.ProviderId, nil
}

//go:generate mockgen -package mocks -destination mocks/exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor
func getNewRunnerExecutor(
	execClient exec.Executor,
	// uniterGetter UnitGetter,
	dataDir string,
) uniter.NewRunnerExecutorFunc {
	return func(providerIDGetter uniter.ProviderIDGetter) runner.ExecFunc {
		return func(params runner.ExecParams) (*utilexec.ExecResponse, error) {
			// podName, err := fetchPodNameForUnit(uniterGetter, unit)
			// if err != nil {
			// 	return nil, errors.Trace(err)
			// }
			if err := providerIDGetter.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
			podNameOrID := providerIDGetter.ProviderID()
			logger.Criticalf("getNewRunnerExecutor podNameOrID -> %q", podNameOrID)

			if err := prepare(execClient, podNameOrID, dataDir, params.Cancel); err != nil {
				return nil, errors.Trace(err)
			}

			if params.Stdout != nil && params.Stderr != nil {
				// run action - stream stdout and stderr to logger.
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
				return &utilexec.ExecResponse{}, nil
			}

			// juju run - return stdout and stderr to ExecResponse.
			var stdout, stderr bytes.Buffer
			if err := execClient.Exec(
				exec.ExecParams{
					PodName:    podNameOrID,
					Commands:   params.Commands,
					WorkingDir: params.WorkingDir,
					Env:        params.Env,
					Stdout:     &stdout,
					Stderr:     &stderr,
				},
				params.Cancel,
			); err != nil {
				return nil, errors.Trace(err)
			}
			return &utilexec.ExecResponse{
				Stdout: stdout.Bytes(),
				Stderr: stderr.Bytes(),
			}, nil
		}
	}
}
