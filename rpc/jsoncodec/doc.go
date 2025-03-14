// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package jsoncodec defines the methods for reading and writing RPC messages (in JSON format)
// and the connection (gorilla websocket) which passes them. It includes:
//
// - Methods for sending and receiving JSON messages.
// - Definitions for input and output message formats (inMsgV0, inMsgV1, outMsgV0, outMsgV1) across protocol versions.
// - Logic to determine the message version (V0/V1) based on the RequestId.

package jsoncodec
