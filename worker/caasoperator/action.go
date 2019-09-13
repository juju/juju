// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils"
	utilexec "github.com/juju/utils/exec"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
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

func ensureSymlink(
	client exec.Executor,
	podName string,
	oldName, newName string,
	stdout io.Writer,
	stderr io.Writer,
	cancel <-chan struct{},
) error {
	logger.Debugf("making symlink %v->%v", newName, oldName)
	err := client.Exec(
		exec.ExecParams{
			PodName:  podName,
			Commands: []string{"test", "-f", newName, "||", "ln", "-s", oldName, newName},
			Stdout:   stdout,
			Stderr:   stderr,
		},
		cancel,
	)
	return errors.Trace(err)
}

type workloadPathSpec struct {
	src, dest string
}

func workloadFilesToCopy(operatorPaths Paths, unitPaths uniter.Paths) []workloadPathSpec {
	return []workloadPathSpec{{
		src:  operatorPaths.GetCharmDir(),
		dest: unitPaths.State.BaseDir,
	}, {
		src:  filepath.Join(operatorPaths.GetToolsDir(), "jujud"),
		dest: unitPaths.ToolsDir,
	}}
}

func prepare(
	client exec.Executor,
	podName string,
	serviceAddress string,
	operatorPaths Paths,
	unitPaths uniter.Paths,
	operatorInfo caas.OperatorInfo,
	stdout io.ReadWriter,
	stderr io.Writer,
	cancel <-chan struct{},
) error {
	// TODO(caas) - quick check to see if files have already been copied across.
	// upgrade-charm and upgrade-juju will need to ensure files are up-to-date.
	operatorFile := filepath.Join(unitPaths.State.BaseDir, caas.OperatorClientInfoFile)
	err := client.Exec(
		exec.ExecParams{
			PodName:  podName,
			Commands: []string{"test", "-f", operatorFile},
			Stdout:   stdout,
			Stderr:   stderr,
		},
		cancel,
	)
	if exitErr, ok := errors.Cause(err).(exec.ExitError); ok {
		if exitErr.ExitStatus() != 1 {
			return errors.Trace(err)
		}
	} else if err != nil {
		return errors.Trace(err)
	} else {
		return nil
	}

	// Copy the core charm files and jujud binary.
	for _, pathSpec := range workloadFilesToCopy(operatorPaths, unitPaths) {
		_, err := os.Stat(pathSpec.src)
		if os.IsNotExist(err) {
			return errors.NotFoundf("file or path %q", pathSpec.src)
		}
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("copy path %q to %q", pathSpec.src, pathSpec.dest)
		if err := ensurePath(client, podName, pathSpec.dest, stdout, stderr, cancel); err != nil {
			return errors.Trace(err)
		}

		if err := client.Copy(exec.CopyParam{
			Src: exec.FileResource{
				Path: pathSpec.src,
			},
			Dest: exec.FileResource{
				Path:    pathSpec.dest,
				PodName: podName,
			},
		}, cancel); err != nil {
			return errors.Trace(err)
		}
	}

	// set up the symlinks to jujud (hook commands and juju-run etc).
	jujudPath := filepath.Join(unitPaths.ToolsDir, "jujud")
	for _, slk := range jujudSymlinks {
		if err := ensureSymlink(client, podName, jujudPath, slk, stdout, stderr, cancel); err != nil {
			return errors.Trace(err)
		}
	}
	for _, cmdName := range jujuc.CommandNames() {
		slk := filepath.Join(unitPaths.ToolsDir, cmdName)
		if err := ensureSymlink(client, podName, jujudPath, slk, stdout, stderr, cancel); err != nil {
			return errors.Trace(err)
		}
	}

	// Ensure unit dir exists for operator-client.yaml and ca.crt file.
	if err := ensurePath(client, podName, unitPaths.State.BaseDir, stdout, stderr, cancel); err != nil {
		return errors.Trace(err)
	}

	// Create the ca.crt file containing the cluster's CA cert.
	tempCACertFile := filepath.Join(os.TempDir(), caas.CACertFile)
	if err := ioutil.WriteFile(tempCACertFile, []byte(operatorInfo.CACert), 0644); err != nil {
		return errors.Trace(err)
	}
	if err := client.Copy(exec.CopyParam{
		Src: exec.FileResource{
			Path: tempCACertFile,
		},
		Dest: exec.FileResource{
			Path:    filepath.Join(unitPaths.State.BaseDir, caas.CACertFile),
			PodName: podName,
		},
	}, cancel); err != nil {
		return errors.Trace(err)
	}

	// Create the operator.yaml file containing the operator service address and token.
	token, err := utils.RandomPassword()
	if err != nil {
		return errors.Trace(err)
	}
	clientInfo := caas.OperatorClientInfo{
		ServiceAddress: serviceAddress,
		Token:          token,
	}
	data, err := clientInfo.Marshal()
	if err != nil {
		return errors.Trace(err)
	}
	operatorCacheFile := filepath.Join(unitPaths.State.BaseDir, caas.OperatorClientInfoCacheFile)
	if err := ioutil.WriteFile(operatorCacheFile, data, 0644); err != nil {
		return errors.Trace(err)
	}
	if err := client.Copy(exec.CopyParam{
		Src: exec.FileResource{
			Path: operatorCacheFile,
		},
		Dest: exec.FileResource{
			Path:    operatorFile,
			PodName: podName,
		},
	}, cancel); err != nil {
		return errors.Trace(err)
	}

	return nil
}

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

			serviceAddress := os.Getenv(provider.OperatorServiceIPEnvName)
			logger.Debugf("operator service address: %v", serviceAddress)
			if err := prepare(
				execClient, podNameOrID, serviceAddress,
				operatorPaths, unitPaths, operatorInfo,
				params.Stdout, params.Stderr, params.Cancel,
			); err != nil {
				logger.Errorf("ensuring dirs and syncing files %v", err)
				return nil, errors.Trace(err)
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
