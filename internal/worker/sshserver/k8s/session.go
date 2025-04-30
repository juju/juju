// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"context"
	"io"

	"github.com/creack/pty"
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
	_, _, ptyRequested := session.Pty()

	var stdin io.Reader = session
	var stdout, stderr io.Writer = session, session.Stderr()

	if ptyRequested {
		// If pty is requested we need to similate a terminal device, passing
		// the pty file descriptor to the executor. And pipe it back to the session.
		ptmx, tty, err := pty.Open()
		if err != nil {
			handleError(errors.Annotate(err, "failed to open pty"))
			return
		}
		defer func() { _ = ptmx.Close() }()
		defer func() { _ = tty.Close() }()

		go func() {
			_, _ = io.Copy(ptmx, session)
		}()
		go func() {
			_, _ = io.Copy(session, ptmx)
		}()

		stdin = tty
		stdout = tty
		stderr = tty
	}

	err = executor.Exec(
		k8sexec.ExecParams{
			PodName:       res.PodName,
			ContainerName: containerName,
			Commands:      session.Command(),
			Stdout:        stdout,
			Stderr:        stderr,
			Stdin:         stdin,
			TTY:           ptyRequested,
			Env:           session.Environ(),
		},
		ctx.Done(),
	)
	if err != nil {
		handleError(errors.Annotate(err, "failed to execute command in k8s pod"))
		return
	}
}
