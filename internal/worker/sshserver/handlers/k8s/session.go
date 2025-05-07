// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"io"
	"os"
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
	var tty, ptmx *os.File

	// If pty is requested we need to simulate a terminal device, passing
	// the pty file descriptor to the executor. And pipe it back to the session.
	// NOTE: we are not sure this is needed, but the bare session is not enough
	// because the file descriptor is not a tty. And when the executor checks for
	// it, it returns an error.
	if ptyRequested {
		ptmx, tty, err = pty.Open()
		if err != nil {
			handleError(errors.Annotate(err, "failed to open pty"))
			return
		}

		defer ptmx.Close()
		defer tty.Close()

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

		wg.Add(2)
		// These goroutines will copy data between the pty and the session.
		// They can't leak because the session is always closed when this
		// function returns.
		go func() {
			defer wg.Done()
			// If the user's session ends, close the ptmx because
			// there is no one listening anymore.
			defer ptmx.Close()
			_, _ = io.Copy(ptmx, session)
		}()
		go func() {
			defer wg.Done()
			// If the ptmx ends, close the session because
			// there is no more data to send.
			defer session.Close()
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
		session.Context().Done(),
	)
	if tty != nil {
		tty.Close()
		// Send a new line to the session to end the master
		// side of the pty.
		_, _ = ptmx.WriteString("\n")
	}
	if err != nil {
		handleError(errors.Annotate(err, "failed to execute command in k8s pod"))
		return
	}
	if ptyRequested {
		// Wait for the goroutines to finish copying data.
		wg.Wait()
	}
}
