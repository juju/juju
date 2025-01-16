// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"fmt"
	"io"
	"net"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/juju/state"
	gossh "golang.org/x/crypto/ssh"
)

// newSSHServer returns an embedded SSH server. This server does a few things,
// reuqests that come in must go through the jump server. The jump server will
// pipe the connection and pass it into an in-memory instance of another SSH server.
//
// This second SSH server (seen within directTCPIPHandlerClosure) will handle the
// the termination of the SSH connections, note, it is not listening on any ports
// because we are passing the piped connection to it, essentially allowing the following
// to work (despite only having one server listening):
// - `ssh -J controller:2223 ubuntu@app.controller.model`
//
// TODO(ale8k): Word this comment better later explaining why the host routing works.
func NewSSHServer(sp *state.StatePool, jumpHostKey, terminatingHostKey string) (*ssh.Server, error) {
	jumpHostSigner, err := gossh.ParsePrivateKey([]byte(jumpHostKey))
	if err != nil {
		return nil, errors.Trace(err)
	}

	terminatingHostSigner, err := gossh.ParsePrivateKey([]byte(terminatingHostKey))
	if err != nil {
		return nil, errors.Trace(err)
	}

	server := &ssh.Server{
		Addr: ":2223",
		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
			return true
		},
		PasswordHandler: func(ctx ssh.Context, password string) bool {
			return true
		},
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"direct-tcpip": directTCPIPHandlerClosure(sp, terminatingHostSigner),
		},
	}

	server.AddHostKey(jumpHostSigner)

	return server, nil
}

// directTCPIPHandlerClosure is a closure that returns a direct-tcpip handler passing in state
// to check the permissions the user has for the model containing this unit.
func directTCPIPHandlerClosure(sp *state.StatePool, terminatingHostSigner gossh.Signer) ssh.ChannelHandler {
	return func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		d := struct {
			DestAddr string
			DestPort uint32
			SrcAddr  string
			SrcPort  uint32
		}{}

		if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
			newChan.Reject(gossh.ConnectionFailed, "Failed to parse channel data")
			return
		}

		ch, reqs, err := newChan.Accept()
		if err != nil {
			return
		}

		go gossh.DiscardRequests(reqs)

		jumpServerPipe, terminatingServerPipe := net.Pipe()

		go func() {
			defer ch.Close()
			defer jumpServerPipe.Close()
			defer terminatingServerPipe.Close()
			io.Copy(ch, jumpServerPipe)
		}()
		go func() {
			defer ch.Close()
			defer jumpServerPipe.Close()
			defer terminatingServerPipe.Close()
			io.Copy(jumpServerPipe, ch)
		}()

		forwardHandler := &ssh.ForwardedTCPHandler{}
		server := &ssh.Server{
			PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
				return true
			},
			LocalPortForwardingCallback: ssh.LocalPortForwardingCallback(func(ctx ssh.Context, dhost string, dport uint32) bool {
				return true
			}),
			ReversePortForwardingCallback: ssh.ReversePortForwardingCallback(func(ctx ssh.Context, host string, port uint32) bool {
				return true
			}),
			ChannelHandlers: map[string]ssh.ChannelHandler{
				"session":      ssh.DefaultSessionHandler,
				"direct-tcpip": ssh.DirectTCPIPHandler,
			},
			RequestHandlers: map[string]ssh.RequestHandler{
				"tcpip-forward":        forwardHandler.HandleSSHRequest,
				"cancel-tcpip-forward": forwardHandler.HandleSSHRequest,
			},
			Handler: func(s ssh.Session) {
				_, _ = s.Write([]byte(fmt.Sprintf("Your final destination is: %s as user: %s\n", d.DestAddr, s.User())))
			},
		}

		server.AddHostKey(terminatingHostSigner)
		server.HandleConn(terminatingServerPipe)
	}
}
