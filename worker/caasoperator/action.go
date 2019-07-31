// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"io"
	"path/filepath"
	"strings"

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
	stdout, stderr io.Writer,
	cancel <-chan struct{},
	path string,
) error {
	err := client.Exec(
		exec.ExecParams{
			PodName:  podName,
			Commands: []string{"mkdir", "-p", path},
			Stdout:   stdout,
			Stderr:   stderr,
		},
		cancel,
	)
	logger.Debugf("ensuring %q", path)
	return errors.Trace(err)
}

type symlink struct {
	Link   string
	Target string
}

// ensureSymlinks ensure a list of symlinks on a pod.
func ensureSymlinks(
	client exec.Executor,
	podName string,
	stdout, stderr io.Writer,
	cancel <-chan struct{},
	links []symlink,
) error {
	var commands []string
	for _, link := range links {
		logger.Debugf("ensuring symlink %q targeting to %q", link.Link, link.Target)
		commands = append(commands,
			strings.Join([]string{"ln", "-s", link.Target, link.Link}, " "),
		)
	}
	err := client.Exec(
		exec.ExecParams{
			PodName:  podName,
			Commands: []string{strings.Join(commands, " && ")},
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

func fetchPodNameForUnit(c UnitGetter, tag names.UnitTag) (string, error) {
	result, err := c.Units(tag)
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
func getNewRunnerExecutor(execClient exec.Executor, uniterGetter UnitGetter, dataDir string) func(unit names.UnitTag, paths uniter.Paths) runner.ExecFunc {
	return func(unit names.UnitTag, paths uniter.Paths) runner.ExecFunc {
		return func(params runner.ExecParams) (*utilexec.ExecResponse, error) {

			podName, err := fetchPodNameForUnit(uniterGetter, unit)
			if err != nil {
				return nil, errors.Trace(err)
			}

			// ensuring data dir.
			if err := ensurePath(execClient, podName, params.Stdout, params.Stderr, params.Cancel, dataDir); err != nil {
				return nil, errors.Trace(err)
			}

			// syncing files.
			// TODO(caas): add a new cmd for checking jujud version, charm/version etc.
			// exec run this new cmd to decide if we need re-push files or not.
			if err := syncFiles(
				execClient, podName, params.Cancel,
				[]string{
					// TODO(caas): only sync files required for actions/run.
					filepath.Join(dataDir, "agents"),
					filepath.Join(dataDir, "tools"),
				},
			); err != nil {
				return nil, errors.Trace(err)
			}

			if err := execClient.Exec(
				exec.ExecParams{
					PodName:    podName,
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
	}
}
