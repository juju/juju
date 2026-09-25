// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"fmt"
	"net"
	"strings"
)

// maxPreBannerLength is the maximum length of the pre-banner message.
// Per RFC 4253 section 4.2 <= 255 bytes.
const maxPreBannerLength = 255

// WritePreBannerError writes msg as SSH pre-banner text (RFC 4253
// section 4.2), which OpenSSH clients display before the version banner.
// The message is flattened to one CRLF-terminated line capped in length.
func WritePreBannerError(conn net.Conn, msg string) error {
	msg = strings.ReplaceAll(msg, "\n", " ")
	if len(msg) > maxPreBannerLength {
		// Reserve 2 bytes for the terminating CR and LF, and cap without
		// splitting a multi-byte rune.
		msg = strings.ToValidUTF8(msg[:maxPreBannerLength-2], "")
	}
	_, err := fmt.Fprintf(conn, "%s\r\n", msg)
	return err
}
