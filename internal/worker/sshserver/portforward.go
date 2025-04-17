// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/state"
)

type portForwardHandler struct {
	connector SSHConnector
	modelType state.ModelType
	logger    Logger
}

// DirectTCPIPHandler returns a handler for the DirectTCPIP channel type
// based on the model type, K8s or machine.
func (p *portForwardHandler) DirectTCPIPHandler(details connectionDetails) ssh.ChannelHandler {
	switch p.modelType {
	case state.ModelTypeCAAS:
		return p.K8sDirectTCPIPHandler(details)
	case state.ModelTypeIAAS:
		return p.MachineDirectTCPIPHandler(details)
	default:
		p.logger.Errorf("unknown model type %s", p.modelType)
		return func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
			_ = newChan.Reject(gossh.ConnectionFailed, "unknown model type")
		}
	}
}

type localForwardChannelData struct {
	DestAddr string
	DestPort uint32

	OriginAddr string
	OriginPort uint32
}

// K8sDirectTCPIPHandler returns a handler for the DirectTCPIP channel type.
// This handler is not currently implemented for Kubernetes models.
func (p *portForwardHandler) K8sDirectTCPIPHandler(details connectionDetails) ssh.ChannelHandler {
	return func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		_ = newChan.Reject(gossh.ConnectionFailed, "local port forwarding unavailable for k8s models")
	}
}

// MachineDirectTCPIPHandler returns a handler for the DirectTCPIP channel type.
// This handler is used for local port forwarding. While the handler is nearly
// identical to the default DirectTCPIPHandler, it first connects to the target
// machine and proxies the port forwarding request through the machine's SSH server.
func (p *portForwardHandler) MachineDirectTCPIPHandler(details connectionDetails) ssh.ChannelHandler {
	return func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		d := localForwardChannelData{}
		if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
			_ = newChan.Reject(gossh.ConnectionFailed, "error parsing forward data: "+err.Error())
			return
		}

		dest := net.JoinHostPort(d.DestAddr, strconv.FormatInt(int64(d.DestPort), 10))

		client, err := p.connector.Connect(details.destination)
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
