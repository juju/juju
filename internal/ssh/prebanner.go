// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"fmt"
	"net"
	"strings"
)

// maxPreBannerLength caps the pre-banner message to one terminal line.
const maxPreBannerLength = 80

// WritePreBannerError writes msg as SSH pre-banner text (RFC 4253
// section 4.2), which OpenSSH clients display before the version banner.
// The message is flattened to one CRLF-terminated line capped in length.
func WritePreBannerError(conn net.Conn, msg string) error {
	msg = strings.ReplaceAll(msg, "\n", " ")
	if len(msg) > maxPreBannerLength {
		// Cap without splitting a multi-byte rune.
		msg = strings.ToValidUTF8(msg[:maxPreBannerLength], "")
	}
	_, err := fmt.Fprintf(conn, "%s\r\n", msg)
	return err
}
