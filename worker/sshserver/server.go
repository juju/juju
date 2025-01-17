// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"
)

// NewSSHServer returns an embedded SSH server.
func NewSSHServer(jumpHostKey string) (*ssh.Server, error) {
	jumpHostSigner, err := gossh.ParsePrivateKey([]byte(jumpHostKey))
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
			"direct-tcpip": directTCPIPHandlerClosure(),
		},
	}

	server.AddHostKey(jumpHostSigner)

	return server, nil
}

// directTCPIPHandlerClosure is a closure that returns a direct-tcpip handler, passing
// in necessary dependencies.
func directTCPIPHandlerClosure() ssh.ChannelHandler {
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

		// gossh.Request are requests sent outside of the normal stream of data (ex. pty-req for an interactive session).
		// Since we only need the raw data to redirect, we can discard them.
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
			// ReversePortForwarding will not be supported.
			ReversePortForwardingCallback: ssh.ReversePortForwardingCallback(func(ctx ssh.Context, host string, port uint32) bool {
				return false
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

		// TODO(ale8k): Update later to generate host keys per unit.
		terminatingHostKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		signer, _ := gossh.NewSignerFromKey(terminatingHostKey)

		server.AddHostKey(signer)
		server.HandleConn(terminatingServerPipe)
	}
}
