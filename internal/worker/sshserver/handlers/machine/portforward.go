// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"
	"io"
	"net"
	"strconv"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
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
// This handler is used for local port forwarding. The newChan is the user's
// SSH channel to the controller, and the handler will create a new connection
// to the target machine and proxy data between the two connections.
func (h *Handlers) DirectTCPIPHandler() ssh.ChannelHandler {
	return func(_ *ssh.Server, _ *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		var data localForwardChannelData
		if err := gossh.Unmarshal(newChan.ExtraData(), &data); err != nil {
			h.logger.Debugf(ctx, "failed to parse local forward channel data: %v", err)
			_ = newChan.Reject(gossh.ConnectionFailed, "parsing forward data: "+err.Error())
			return
		}

		destination := net.JoinHostPort(data.DestAddr, strconv.FormatUint(uint64(data.DestPort), 10))
		accepted := false
		handleProxy(h, ctx, proxyConfig[halfCloseConn]{
			createRemote: func(ctx context.Context, client *gossh.Client) (halfCloseConn, error) {
				connection, err := client.DialContext(ctx, "tcp", destination)
				if err != nil {
					return nil, err
				}
				halfCloseConnection, ok := connection.(halfCloseConn)
				if !ok {
					_ = connection.Close()
					return nil, errors.New("target connection does not support half-close")
				}

				return halfCloseConnection, nil
			},
			run: func(remote halfCloseConn) error {
				channel, requests, err := newChan.Accept()
				if err != nil {
					return err
				}
				accepted = true
				stop := context.AfterFunc(ctx, func() {
					_ = channel.Close()
				})
				defer stop()

				go gossh.DiscardRequests(requests)
				proxyStreams(channel, remote)
				_ = channel.Close()
				return nil
			},
			onError: func(err error) {
				h.logger.Debugf(ctx, "machine proxy failure: %v", err)
				if accepted {
					return
				}
				_ = newChan.Reject(gossh.ConnectionFailed, err.Error())
			},
		})
	}
}
