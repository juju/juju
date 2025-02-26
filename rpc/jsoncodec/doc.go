// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package jsoncodec defines the methods for reading and writing RPC messages, enabling serialization and deserialization.

// Key components

// - Implements methods for sending and receiving JSON messages.
// - Defines message formats (inMsgV0, inMsgV1, outMsgV0, outMsgV1) for different protocol versions.
// - Uses a mutex to handle concurrent access and prevent race conditions.
// - Determines version based on the presence of specific JSON fields (e.g. RequestId)

package jsoncodec
