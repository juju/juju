// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/errors"
)

// Port identifies a network port number for a particular protocol.
//
// NOTE(dimitern): This is deprecated and should be removed, use
// PortRange instead. There are a few places which still use Port,
// especially in apiserver/params, so it can't be removed yet.
type Port struct {
	Protocol string
	Number   int
}

// String implements Stringer.
func (p Port) String() string {
	return fmt.Sprintf("%d/%s", p.Number, p.Protocol)
}

// GoString implements fmt.GoStringer.
func (p Port) GoString() string {
	return p.String()
}

// ParsePortRange builds a PortRange from the provided string. If the
// string does not include a protocol then "tcp" is used. Validate()
// gets called on the result before returning. If validation fails the
// invalid PortRange is still returned.
// Example strings: "80/tcp", "443", "12345-12349/udp".
func ParsePortRange(portRangeStr string) (*PortRange, error) {
	// Extract the protocol.
	protocol := "tcp"
	parts := strings.SplitN(portRangeStr, "/", 2)
	if len(parts) == 2 {
		portRangeStr = parts[0]
		protocol = parts[1]
	}

	// Parse the ports.
	portRange, err := parsePortRange(portRangeStr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	portRange.Protocol = protocol

	return portRange, portRange.Validate()
}

func parsePortRange(portRangeStr string) (*PortRange, error) {
	var start, end int
	parts := strings.Split(portRangeStr, "-")
	if len(parts) > 2 {
		return nil, errors.Errorf("invalid port range %q", portRangeStr)
	}

	if len(parts) == 1 {
		port, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, errors.Annotatef(err, "invalid port %q", portRangeStr)
		}
		start = port
		end = port
	} else {
		var err error
		if start, err = strconv.Atoi(parts[0]); err != nil {
			return nil, errors.Annotatef(err, "invalid port %q", parts[0])
		}
		if end, err = strconv.Atoi(parts[1]); err != nil {
			return nil, errors.Annotatef(err, "invalid port %q", parts[1])
		}
	}

	result := PortRange{
		FromPort: start,
		ToPort:   end,
	}
	return &result, nil
}
