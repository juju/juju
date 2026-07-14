// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshconn provides an adapter that exposes an SSH channel
// (golang.org/x/crypto/ssh) as a net.Conn. It is used to treat an opened SSH
// channel as a plain network connection so that it can be piped to another
// connection, such as the local sshd, when establishing reverse SSH tunnels.
package sshconn
