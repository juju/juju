// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshconn contains a struct from golang.org/x/crypto/ssh that converts
// ssh.Channel into a net.Conn. Use where both client and server agree that
// the channel will act as a raw tcp connection.
package sshconn
