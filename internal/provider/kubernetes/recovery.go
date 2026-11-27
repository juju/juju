// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	internalerrors "github.com/juju/juju/internal/errors"
	k8sexec "github.com/juju/juju/internal/provider/kubernetes/exec"
)

const recoveryAgentPollDelay = 500 * time.Millisecond

// recoveryPodSetup builds the controller-0 startup shell snippet for a
// recovery allocation. It creates the recovery parameters marker only on
// initial boot (before agent.conf or the marker exist), runs recover-state
// until the running agent acknowledges the import fence, and propagates any
// setup failure so Pebble never starts after an incomplete setup. Once the
// agent writes the waiting acknowledgement or recovery removes the recovery
// directory, subsequent pod restarts skip setup and start Pebble directly.
func recoveryPodSetup(agentConfigPath, bootstrapStateCmd string) string {
	paramsPath := `"` + path.Join("$JUJU_DATA_DIR", agent.RecoveryParamsFile) + `"`
	waitingPath := `"` + path.Join("$JUJU_DATA_DIR", agent.RecoveryAgentWaitingFile) + `"`
	quotedConfigPath := `"` + agentConfigPath + `"`
	return fmt.Sprintf(
		"if ! test -e %s && ! test -e %s; then %s || exit $?; fi; "+
			"if test -e %s && ! test -e %s; then %s || exit $?; fi",
		quotedConfigPath,
		paramsPath,
		cloudconfig.RecoveryMarkerCommand(paramsPath),
		paramsPath,
		waitingPath,
		bootstrapStateCmd,
	)
}

// WaitForRecoveryAgent waits until the replacement Kubernetes controller agent
// has observed its recovery import fence, or until startup fails, times out or
// is cancelled.
func (k *kubernetesClient) WaitForRecoveryAgent(
	ctx context.Context,
	pcfg *podcfg.ControllerPodConfig,
) error {
	execClient, err := k.controllerExecClient()
	if err != nil {
		return internalerrors.Capture(err)
	}
	return waitForRecoveryAgent(ctx, execClient, pcfg)
}

// waitForRecoveryAgent polls the replacement controller's api-server container
// until both the recovery marker and the agent's wait acknowledgement exist as
// regular files, or until a non-retryable exec error, timeout or cancellation
// occurs.
func waitForRecoveryAgent(
	ctx context.Context,
	execClient k8sexec.Executor,
	pcfg *podcfg.ControllerPodConfig,
) error {
	if pcfg == nil || pcfg.Bootstrap == nil {
		return errors.NotValidf("missing controller pod bootstrap config")
	}
	if pcfg.DataDir == "" {
		return errors.NotValidf("missing controller pod data directory")
	}

	podName := pcfg.GetPodName()
	probe := []string{
		"test", "-f", path.Join(pcfg.DataDir, agent.RecoveryParamsFile),
		"&&",
		"test", "-f", path.Join(pcfg.DataDir, agent.RecoveryAgentWaitingFile),
	}

	waitCtx, cancel := context.WithTimeout(ctx, pcfg.Bootstrap.Timeout)
	defer cancel()

	for {
		if err := waitCtx.Err(); err != nil {
			return internalerrors.Capture(err)
		}
		var stderr bytes.Buffer
		err := execClient.Exec(waitCtx, k8sexec.ExecParams{
			PodName:       podName,
			ContainerName: apiServerContainerName,
			Commands:      probe,
			Stderr:        &stderr,
		}, nil)
		if err == nil {
			return nil
		}
		if waitErr := waitCtx.Err(); waitErr != nil {
			return internalerrors.Capture(waitErr)
		}
		if !isRetryableRecoveryProbeError(err) {
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				return internalerrors.Errorf("%s: %w", msg, err)
			}
			return internalerrors.Capture(err)
		}

		timer := time.NewTimer(recoveryAgentPollDelay)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return internalerrors.Capture(waitCtx.Err())
		case <-timer.C:
		}
	}
}

// isRetryableRecoveryProbeError reports whether err from the recovery fence
// probe indicates that the container is still starting or the markers are not
// present yet, as opposed to an exec or permission failure.
func isRetryableRecoveryProbeError(err error) bool {
	if k8sexec.IsContainerNotRunningError(err) {
		return true
	}
	var notRunning *k8sexec.ContainerNotRunningError
	if internalerrors.As(err, &notRunning) {
		return true
	}
	var exitErr k8sexec.ExitError
	if internalerrors.As(err, &exitErr) {
		return exitErr.ExitStatus() == 1
	}
	if exitErr, ok := errors.Cause(err).(k8sexec.ExitError); ok {
		return exitErr.ExitStatus() == 1
	}
	return false
}
