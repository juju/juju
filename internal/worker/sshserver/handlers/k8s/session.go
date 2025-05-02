// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"context"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"

	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/rpc/params"
)

// SessionHandler proxies a user's SSH session to a K8s container.
func (h *Handlers) SessionHandler(session ssh.Session) {
	handleError := func(err error) {
		h.logger.Errorf("k8s session handler failure: %v", err)
		_, _ = session.Stderr().Write([]byte(err.Error() + "\n"))
		_ = session.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	unitName, _ := h.destination.Unit()
	res, err := h.resolver.ResolveK8sExecInfo(params.SSHK8sExecArg{
		UnitName:  unitName,
		ModelUUID: h.destination.ModelUUID(),
	})
	if err != nil {
		handleError(errors.Annotate(err, "failed to resolve k8s exec info"))
		return
	}

	executor, err := h.getExecutor(res.Namespace)
	if err != nil {
		handleError(errors.Annotate(err, "failed to get executor"))
		return
	}
	containerName, _ := h.destination.Container()
	// TODO(JUJU-7880): improve pty sessions
	_, _, pty := session.Pty()
	err = executor.Exec(
		k8sexec.ExecParams{
			PodName:       res.PodName,
			ContainerName: containerName,
			Commands:      session.Command(),
			Stdout:        session,
			Stderr:        session.Stderr(),
			Stdin:         session,
			TTY:           pty,
			Env:           session.Environ(),
		},
		ctx.Done(),
	)
	if err != nil {
		handleError(errors.Annotate(err, "failed to execute command in k8s pod"))
		return
	}
}
