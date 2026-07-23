// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// localForwardChannelData mirrors the unexported
// gossh.localForwardChannelData from x/crypto/ssh.
type localForwardChannelData struct {
	DestAddr   string
	DestPort   uint32
	OriginAddr string
	OriginPort uint32
}

type halfCloseConn interface {
	io.ReadWriteCloser
	CloseWrite() error
}

// DirectTCPIPHandler returns a handler for the DirectTCPIP channel type.
// This handler is used for local port forwarding. While the handler is similar
// to the default DirectTCPIPHandler, it first connects to the target machine
// machine and proxies the port forwarding request through the machine's SSH server.
func (h *Handlers) DirectTCPIPHandler() ssh.ChannelHandler {
	return func(_ *ssh.Server, _ *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		var data localForwardChannelData
		if err := gossh.Unmarshal(newChan.ExtraData(), &data); err != nil {
			h.logger.Debugf(ctx, "failed to parse local forward channel data: %v", err)
			_ = newChan.Reject(gossh.ConnectionFailed, "parsing forward data: "+err.Error())
			return
		}

		client, err := h.connector.Connect(ctx, h.destination)
		if err != nil {
			h.logger.Debugf(ctx, "failed to connect to machine: %v", err)
			_ = newChan.Reject(gossh.ConnectionFailed, fmt.Sprintf("connecting to machine: %v", err))
			return
		}
		defer client.Close()

		destination := net.JoinHostPort(data.DestAddr, strconv.FormatUint(uint64(data.DestPort), 10))
		connection, err := client.DialContext(ctx, "tcp", destination)
		if err != nil {
			h.logger.Debugf(ctx, "failed to dial target %q: %v", destination, err)
			_ = newChan.Reject(gossh.ConnectionFailed, fmt.Sprintf("dialling target: %v", err))
			return
		}

		halfCloseConnection, ok := connection.(halfCloseConn)
		if !ok {
			h.logger.Debugf(ctx, "target connection does not support half-close")
			_ = connection.Close()
			_ = newChan.Reject(gossh.ConnectionFailed, "target connection does not support half-close")
			return
		}

		channel, requests, err := newChan.Accept()
		if err != nil {
			h.logger.Debugf(ctx, "failed to accept channel: %v", err)
			_ = connection.Close()
			return
		}
		defer channel.Close()
		defer halfCloseConnection.Close()

		stop := context.AfterFunc(ctx, func() {
			_ = channel.Close()
			_ = halfCloseConnection.Close()
		})
		defer stop()

		go gossh.DiscardRequests(requests)
		proxy(channel, halfCloseConnection)
	}
}

func proxy(channel gossh.Channel, connection halfCloseConn) {
	var wg sync.WaitGroup
	wg.Go(func() {
		_, _ = io.Copy(channel, connection)
		_ = channel.CloseWrite()
	})
	wg.Go(func() {
		_, _ = io.Copy(connection, channel)
		_ = connection.CloseWrite()
	})
	wg.Wait()
}
