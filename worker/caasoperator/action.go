// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"bytes"

	"github.com/juju/clock"
	"github.com/juju/errors"
	utilexec "github.com/juju/utils/exec"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
)

func ensurePath(client exec.Executor, podName string, stdout, stderr bytes.Buffer, cancel <-chan struct{}) error {
	err := client.Exec(
		exec.ExecParams{
			PodName:  podName,
			Commands: []string{"mkdir", "-p", "/var/lib/juju"},
			Stdout:   &stdout,
			Stderr:   &stderr,
		},
		cancel,
	)
	return errors.Trace(err)
}

func syncFiles(client exec.Executor, podName string, stdout, stderr bytes.Buffer, cancel <-chan struct{}) error {
	// TODO(caas): add a new cmd for checking jujud version, charm/version etc.
	// exec run this new cmd to decide if we need repush files or not.
	for _, sync := range []exec.CopyParam{
		{
			Src: exec.FileResource{
				Path: "/var/lib/juju/agents/",
			},
			Dest: exec.FileResource{
				Path:    "/var/lib/juju/agents/",
				PodName: podName,
			},
		},
		{
			Src: exec.FileResource{
				Path: "/var/lib/juju/tools/",
			},
			Dest: exec.FileResource{
				Path:    "/var/lib/juju/tools/",
				PodName: podName,
			},
		},
	} {
		if err := client.Copy(sync, cancel); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func fetchPodNameForUnit(c UnitGetter, tag names.UnitTag) (string, error) {
	result, err := c.Units(tag)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(result.Results) == 0 {
		return "", errors.NotFoundf("unit %q", tag.Id())
	}
	unit := result.Results[0]
	logger.Criticalf("fetchPodNameForUnit unit.Result -> %v, unit.Error -> %v", unit.Result, unit.Error)
	if unit.Error != nil {
		return "", unit.Error
	}
	return unit.Result.ProviderId, nil
}

//go:generate mockgen -package mocks -destination mocks/exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor
func getNewRunnerExecutor(execClient exec.Executor, uniterGetter UnitGetter) func(unit names.UnitTag) runner.ExecFunc {
	return func(unit names.UnitTag) runner.ExecFunc {
		return func(
			commands []string,
			env []string,
			workingDir string,
			clock clock.Clock,
			_ func(context.HookProcess),
			cancel <-chan struct{},
		) (*utilexec.ExecResponse, error) {

			podName, err := fetchPodNameForUnit(uniterGetter, unit)
			if err != nil {
				return nil, errors.Trace(err)
			}
			logger.Criticalf("fetchPodNameForUnit podName -> %v", podName)

			var stdout, stderr bytes.Buffer
			if err := ensurePath(execClient, podName, stdout, stderr, cancel); err != nil {
				logger.Errorf("ensuring /var/lib/juju %q", stderr.String())
				return nil, errors.Trace(err)
			}
			logger.Debugf("ensuring /var/lib/juju %q", stdout.String())

			if err := syncFiles(execClient, podName, stdout, stderr, cancel); err != nil {
				logger.Errorf("syncing files %q", stderr.String())
				return nil, errors.Trace(err)
			}
			logger.Debugf("syncing files %q", stdout.String())
			if err := execClient.Exec(
				exec.ExecParams{
					PodName:    podName,
					Commands:   commands,
					WorkingDir: workingDir,
					Env:        env,
					Stdout:     &stdout,
					Stderr:     &stderr,
				},
				cancel,
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
