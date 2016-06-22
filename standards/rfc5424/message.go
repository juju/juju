// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424

import (
	"fmt"
	"net"
	"time"
)

// ProtocolVersion is the syslog protocol version implemented in
// this package.
const ProtocolVersion = 1

// Message holds a single RFC-5424 log record.
//
// See https://tools.ietf.org/html/rfc5424#section-6.
type Message struct {
	Header
	StructuredData

	// Msg is the record's UTF-8 message string.
	Msg string
}

// String returns the RFC 5424 representation of the log record.
func (m Message) String() string {
	if m.Msg == "" {
		return fmt.Sprintf("%s %s", m.Header, m.StructuredData)
	}
	return fmt.Sprintf("%s %s %s", m.Header, m.StructuredData, m.Msg)
}

// Validate ensures that the record is correct.
func (m Message) Validate() error {
	if err := m.Header.Validate(); err != nil {
		return fmt.Errorf("bad Header: %v", err)
	}
	if err := m.StructuredData.Validate(); err != nil {
		return fmt.Errorf("bad StructuredData: %v", err)
	}
	if err := validateUTF8(m.Msg); err != nil {
		return fmt.Errorf("bad Msg: %v", err)
	}
	return nil
}

// Header holds the header portion of the log record.
type Header struct {
	Priority

	// Timestamp indicates when the record was originally created.
	Timestamp Timestamp

	// Hostname identifies the machine that originally sent the
	// syslog message.
	Hostname Hostname

	// AppName identifies the device or application that originated
	// the syslog message.
	AppName AppName

	// ProcID is a value that is included in the message, having no
	// interoperable meaning, except that a change in the value
	// indicates there has been a discontinuity in syslog reporting.
	// The field does not have any specific syntax or semantics; the
	// value is implementation-dependent and/or operator-assigned.
	ProcID ProcID

	// MsgID identifies the type of message. Messages with the same
	// MsgID should reflect events with the same semantics. The MSGID
	// itself is a string without further semantics. It is intended
	// for filtering messages on a relay or collector.
	MsgID MsgID
}

// String returns an RFC 5424 representation of the header.
func (h Header) String() string {
	return fmt.Sprintf("%s%d %s %s %s %s %s", h.Priority, ProtocolVersion, h.Timestamp, h.Hostname, h.AppName, h.ProcID, h.MsgID)
}

// Validate ensures that the header is correct.
func (h Header) Validate() error {
	if err := h.Priority.Validate(); err != nil {
		return fmt.Errorf("bad Priority: %v", err)
	}
	if err := h.Hostname.Validate(); err != nil {
		return fmt.Errorf("bad Hostname: %v", err)
	}
	if err := h.AppName.Validate(); err != nil {
		return fmt.Errorf("bad AppName: %v", err)
	}
	if err := h.ProcID.Validate(); err != nil {
		return fmt.Errorf("bad ProcID: %v", err)
	}
	if err := h.MsgID.Validate(); err != nil {
		return fmt.Errorf("bad MsgID: %v", err)
	}
	return nil
}

// Timestamp is an RFC 5424 timestamp.
type Timestamp struct {
	time.Time
}

// String returns the RFC 5424 representation of the timestamp. In
// particular, this is RFC 3339 with some restrictions and a special
// case of "-" for the zero value.
func (t Timestamp) String() string { // essentially RFC 3339
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339Nano)
}

var zeroIP net.IP

// Hostname hold the different possible values for an RFC 5424 value.
// The first non-empty field is the one that gets used.
type Hostname struct {
	// FQDN is a fully-qualified domain name.
	//
	// See RFC 1034.
	FQDN string

	// StaticIP is a statically-assigned IP address.
	//
	// See RFC 1035 or 4291-2.2.
	StaticIP net.IP

	// Hostname is an unqualified host name.
	Hostname string

	// DyanmicIP is a dynamically-assigned IP address.
	//
	// See RFC 1035 or 4291-2.2.
	DynamicIP net.IP
}

// String is the RFC 5424 representation of the hostname.
func (h Hostname) String() string {
	switch {
	case h.FQDN != "":
		return h.FQDN
	case !h.StaticIP.Equal(zeroIP):
		return h.StaticIP.String()
	case h.Hostname != "":
		return h.Hostname
	case !h.DynamicIP.Equal(zeroIP):
		return h.DynamicIP.String()
	default:
		return "-"
	}
}

// Validate ensures that the hostname is correct.
func (h Hostname) Validate() error {
	switch {
	case h.FQDN != "":
		// TODO(ericsnow) finish
	case !h.StaticIP.Equal(zeroIP):
		// TODO(ericsnow) finish
	case h.Hostname != "":
		// TODO(ericsnow) finish
	case !h.DynamicIP.Equal(zeroIP):
		// TODO(ericsnow) finish
	default:
		return nil
	}

	return validatePrintUSASCII(h.String(), 255)
}

// AppName is the name of the originating app or device.
type AppName string

// String is the RFC 5424 representation of the app name.
func (an AppName) String() string {
	if len(an) == 0 {
		return "-"
	}
	return string(an)
}

// Validate ensures the that app name is correct.
func (an AppName) Validate() error {
	if an == "-" {
		return fmt.Errorf(`"-" is reserved`)
	}
	return validatePrintUSASCII(string(an), 48)
}

// ProcID identifies a group of syslog messages.
type ProcID string

// String is the RFC representation of the proc ID.
func (pid ProcID) String() string {
	if len(pid) == 0 {
		return "-"
	}
	return string(pid)
}

// Validate ensures that the proc ID is correct.
func (pid ProcID) Validate() error {
	if pid == "-" {
		return fmt.Errorf(`"-" is reserved`)
	}
	return validatePrintUSASCII(string(pid), 128)
}

// MsgID identifies a syslog message type.
type MsgID string

// String is the RFC representation of the message ID.
func (mid MsgID) String() string {
	if len(mid) == 0 {
		return "-"
	}
	return string(mid)
}

// Validate ensures that the message ID is correct.
func (mid MsgID) Validate() error {
	if mid == "-" {
		return fmt.Errorf(`"-" is reserved`)
	}
	return validatePrintUSASCII(string(mid), 32)
}
