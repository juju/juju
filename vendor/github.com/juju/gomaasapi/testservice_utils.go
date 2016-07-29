// Copyright 2012-2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
)

// NameOrIDToID takes a string that contains eiter an integer ID or the
// name of a thing. It returns the integer ID contained or mapped to or panics.
func NameOrIDToID(v string, nameToID map[string]uint, minID, maxID uint) (ID uint, err error) {
	ID, ok := nameToID[v]
	if !ok {
		intID, err := strconv.Atoi(v)
		if err != nil {
			return 0, err
		}
		ID = uint(intID)
	}

	if ID < minID || ID > maxID {
		return 0, errors.New("ID out of range")
	}

	return ID, nil
}

// IP is an enhanced net.IP
type IP struct {
	netIP   net.IP
	Purpose []string
}

// IPFromNetIP creates a IP from a net.IP.
func IPFromNetIP(netIP net.IP) IP {
	var ip IP
	ip.netIP = netIP
	return ip
}

// IPFromString creates a new IP from a string IP address representation
func IPFromString(v string) IP {
	return IPFromNetIP(net.ParseIP(v))
}

// IPFromInt64 creates a new IP from a uint64 IP address representation
func IPFromInt64(v uint64) IP {
	var ip IP
	ip.SetUInt64(v)
	return ip
}

// To4 converts the IPv4 address ip to a 4-byte representation. If ip is not
// an IPv4 address, To4 returns nil.
func (ip IP) To4() net.IP {
	return ip.netIP.To4()
}

// To16 converts the IP address ip to a 16-byte representation. If ip is not
// an IP address (it is the wrong length), To16 returns nil.
func (ip IP) To16() net.IP {
	return ip.netIP.To16()
}

func (ip IP) String() string {
	return ip.netIP.String()
}

// UInt64 returns a uint64 holding the IP address
func (ip IP) UInt64() uint64 {
	if len(ip.netIP) == 0 {
		return uint64(0)
	}

	if ip.To4() != nil {
		return uint64(binary.BigEndian.Uint32([]byte(ip.To4())))
	}

	return binary.BigEndian.Uint64([]byte(ip.To16()))
}

// SetUInt64 sets the IP value to v
func (ip *IP) SetUInt64(v uint64) {
	if len(ip.netIP) == 0 {
		// If we don't have allocated storage make an educated guess
		// at if the address we received is an IPv4 or IPv6 address.
		if v == (v & 0x00000000ffffFFFF) {
			// Guessing IPv4
			ip.netIP = net.ParseIP("0.0.0.0")
		} else {
			ip.netIP = net.ParseIP("2001:4860:0:2001::68")
		}
	}

	bb := new(bytes.Buffer)
	var first int
	if ip.To4() != nil {
		binary.Write(bb, binary.BigEndian, uint32(v))
		first = len(ip.netIP) - 4
	} else {
		binary.Write(bb, binary.BigEndian, v)
	}
	copy(ip.netIP[first:], bb.Bytes())
}

func PrettyJsonWriter(thing interface{}, w http.ResponseWriter) {
	var out bytes.Buffer
	b, err := json.MarshalIndent(thing, "", "  ")
	checkError(err)
	out.Write(b)
	out.WriteTo(w)
}
