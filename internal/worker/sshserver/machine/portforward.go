// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

type localForwardChannelData struct {
	DestAddr string
	DestPort uint32

	OriginAddr string
	OriginPort uint32
}

// DirectTCPIPHandler returns a handler for the DirectTCPIP channel type.
// This handler is used for local port forwarding. While the handler is nearly
// identical to the default DirectTCPIPHandler, it first connects to the target
// machine and proxies the port forwarding request through the machine's SSH server.
func (m *Handlers) DirectTCPIPHandler() ssh.ChannelHandler {
	return func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		d := localForwardChannelData{}
		if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
			_ = newChan.Reject(gossh.ConnectionFailed, "error parsing forward data: "+err.Error())
			return
		}

		dest := net.JoinHostPort(d.DestAddr, strconv.FormatInt(int64(d.DestPort), 10))

		client, err := m.connector.Connect(m.destination)
		if err != nil {
			_ = newChan.Reject(gossh.ConnectionFailed, fmt.Sprintf("failed to connect to machine: %s", err.Error()))
			return
		}
		dconn, err := client.DialContext(ctx, "tcp", dest)
		if err != nil {
			_ = newChan.Reject(gossh.ConnectionFailed, fmt.Sprintf("failed to dial target: %s", err.Error()))
			return
		}

		ch, reqs, err := newChan.Accept()
		if err != nil {
			dconn.Close()
			return
		}
		go gossh.DiscardRequests(reqs)

		go func() {
			defer ch.Close()
			defer dconn.Close()
			_, _ = io.Copy(ch, dconn)
		}()
		go func() {
			defer ch.Close()
			defer dconn.Close()
			_, _ = io.Copy(dconn, ch)
		}()
	}
}
