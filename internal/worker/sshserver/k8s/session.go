// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"fmt"
	"io"
	"sync"

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
	ptyReq, winCh, ptyRequested := session.Pty()

	var stdin io.Reader = session
	var stdout, stderr io.Writer = session, session.Stderr()
	wg := &sync.WaitGroup{}

	ptmx, tty, err := pty.Open()
	if ptyRequested {
		// If pty is requested we need to simulate a terminal device, passing
		// the pty file descriptor to the executor. And pipe it back to the session.
		if err != nil {
			handleError(errors.Annotate(err, "failed to open pty"))
			return
		}
		wg.Add(2)

		err = pty.Setsize(ptmx, &pty.Winsize{
			Rows: uint16(ptyReq.Window.Height),
			Cols: uint16(ptyReq.Window.Width),
		})
		if err != nil {
			handleError(errors.Annotate(err, "failed to set pty size"))
			return
		}

		// Listen for window size changes. When the session is closed,
		// the channel will be closed as well.
		go func() {
			for win := range winCh {
				_ = pty.Setsize(ptmx, &pty.Winsize{
					Rows: uint16(win.Height),
					Cols: uint16(win.Width),
				})
			}
		}()

		go func() {
			defer wg.Done()
			defer ptmx.Close()
			defer tty.Close()
			_, err = io.Copy(ptmx, session)
			fmt.Print(err)
		}()

		go func() {
			defer wg.Done()
			defer ptmx.Close()
			defer tty.Close()
			_, err = io.Copy(session, ptmx)
			fmt.Print(err)

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
		session.Context().Done(),
	)
	if err != nil {
		handleError(errors.Annotate(err, "failed to execute command in k8s pod"))
		return
	}
	if ptyRequested {
		err := ptmx.Close()
		fmt.Print(err)
		wg.Wait()
		fmt.Print("")
	}
}
