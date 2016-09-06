// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"bufio"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
)

// DNSConfig holds a list of DNS nameserver addresses and default search
// domains.
type DNSConfig struct {
	Nameservers   []Address
	SearchDomains []string
}

// ParseResolvConf parses a resolv.conf(5) file at the given path (usually
// "/etc/resolv.conf"), if present. Returns the values of any 'nameserver'
// stanzas, and the last 'search' stanza found. Values in the result will appear
// in the order found, including duplicates. Parsing errors will be returned in
// these cases:
//
// 1. if a 'nameserver' or 'search' without a value is found;
// 2. 'nameserver' with more than one value (trailing comments starting with '#'
//    or ';' after the value are allowed).
// 3. if any value containing '#' or ';' (e.g. 'nameserver 8.8.8.8#bad'), because
//    values and comments following them must be separated by whitespace.
//
// No error is returned if the file is missing. See resolv.conf(5) man page for
// details.
func ParseResolvConf(path string) (*DNSConfig, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		logger.Debugf("%q does not exist - not parsing", path)
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	defer file.Close()

	var (
		nameservers   []string
		searchDomains []string
	)
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		values, err := parseResolvStanza(line, "nameserver")
		if err != nil {
			return nil, errors.Annotatef(err, "parsing %q, line %d", path, lineNum)
		}

		if numValues := len(values); numValues > 1 {
			return nil, errors.Errorf(
				"parsing %q, line %d: one value expected for \"nameserver\", got %d",
				path, lineNum, numValues,
			)
		} else if numValues == 1 {
			nameservers = append(nameservers, values[0])
			continue
		}

		values, err = parseResolvStanza(line, "search")
		if err != nil {
			return nil, errors.Annotatef(err, "parsing %q, line %d", path, lineNum)
		}

		if len(values) > 0 {
			// Last 'search' found wins.
			searchDomains = values
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, errors.Annotatef(err, "reading %q", path)
	}

	return &DNSConfig{
		Nameservers:   NewAddresses(nameservers...),
		SearchDomains: searchDomains,
	}, nil
}

// parseResolvStanza parses a single line from a resolv.conf(5) file, beginning
// with the given stanza ('nameserver' or 'search' ). If the line does not
// contain the stanza, no results and no error is returned. Leading and trailing
// whitespace is removed first, then lines starting with ";" or "#" are treated
// as comments.
//
// Examples:
// parseResolvStanza(`   # nothing ;to see here`, "doesn't matter")
// will return (nil, nil) - comments and whitespace are ignored, nothing left.
//
// parseResolvStanza(`   nameserver    ns1.example.com   # preferred`, "nameserver")
// will return ([]string{"ns1.example.com"}, nil).
//
// parseResolvStanza(`search ;; bad: no value`, "search")
// will return (nil, err: `"search": required value(s) missing`)
//
// parseResolvStanza(`search foo bar foo foo.bar bar.foo ;; try all`, "search")
// will return ([]string("foo", "bar", "foo", "foo.bar", "bar.foo"}, nil)
//
// parseResolvStanza(`search foo#bad comment`, "nameserver")
// will return (nil, nil) - line does not start with "nameserver".
//
// parseResolvStanza(`search foo#bad comment`, "search")
// will return (nil, err: `"search": invalid value "foo#bad"`) - no whitespace
// between the value "foo" and the following comment "#bad comment".
func parseResolvStanza(line, stanza string) ([]string, error) {
	const commentChars = ";#"
	isComment := func(s string) bool {
		return strings.IndexAny(s, commentChars) == 0
	}

	line = strings.TrimSpace(line)
	fields := strings.Fields(line)
	noFields := len(fields) == 0 // line contains only whitespace

	if isComment(line) || noFields || fields[0] != stanza {
		// Lines starting with ';' or '#' are comments and are ignored. Empty
		// lines and those not starting with stanza are ignored.
		return nil, nil
	}

	// Mostly for convenience, comments starting with ';' or '#' after a value
	// are allowed and ignored, assuming there's whitespace between the value
	// and the comment (e.g. 'search foo #bar' is OK, but 'search foo#bar'
	// isn't).
	var parsedValues []string
	rawValues := fields[1:] // skip the stanza itself
	for _, value := range rawValues {
		if isComment(value) {
			// We're done parsing as the rest of the line is still part of the
			// same comment.
			break
		}

		if strings.ContainsAny(value, commentChars) {
			// This will catch cases like 'nameserver 8.8.8.8#foo', because
			// fields[1] will be '8.8.8.8#foo'.
			return nil, errors.Errorf("%q: invalid value %q", stanza, value)
		}

		parsedValues = append(parsedValues, value)
	}

	// resolv.conf(5) states that to be recognized as valid, the line must begin
	// with the stanza, followed by whitespace, then at least one value (for
	// 'nameserver', more values separated by whitespace are allowed for
	// 'search').
	if len(parsedValues) == 0 {
		return nil, errors.Errorf("%q: required value(s) missing", stanza)
	}

	return parsedValues, nil
}

var netListen = net.Listen

// SupportsIPv6 reports whether the platform supports IPv6 networking
// functionality.
//
// Source: https://github.com/golang/net/blob/master/internal/nettest/stack.go
func SupportsIPv6() bool {
	ln, err := netListen("tcp6", "[::1]:0")
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// SysClassNetRoot is the full Linux SYSFS path containing information about
// each network interface on the system. Used as argument to
// ParseInterfaceType().
const SysClassNetPath = "/sys/class/net"

// ParseInterfaceType parses the DEVTYPE attribute from the Linux kernel
// userspace SYSFS location "<sysPath/<interfaceName>/uevent" and returns it as
// InterfaceType. SysClassNetPath should be passed as sysPath. Returns
// UnknownInterface if the type cannot be reliably determined for any reason.
//
// Example call: network.ParseInterfaceType(network.SysClassNetPath, "br-eth1")
func ParseInterfaceType(sysPath, interfaceName string) InterfaceType {
	const deviceType = "DEVTYPE="
	location := filepath.Join(sysPath, interfaceName, "uevent")

	data, err := ioutil.ReadFile(location)
	if err != nil {
		logger.Debugf("ignoring error reading %q: %v", location, err)
		return UnknownInterface
	}

	devtype := ""
	lines := strings.Fields(string(data))
	for _, line := range lines {
		if !strings.HasPrefix(line, deviceType) {
			continue
		}

		devtype = strings.TrimPrefix(line, deviceType)
		switch devtype {
		case "bridge":
			return BridgeInterface
		case "vlan":
			return VLAN_8021QInterface
		case "bond":
			return BondInterface
		case "":
			// DEVTYPE is not present for some types, like Ethernet and loopback
			// interfaces, so if missing do not try to guess.
			break
		}
	}

	return UnknownInterface
}

// GetBridgePorts extracts and returns the names of all interfaces configured as
// ports of the given bridgeName from the Linux kernel userspace SYSFS location
// "<sysPath/<bridgeName>/brif/*". SysClassNetPath should be passed as sysPath.
// Returns an empty result if the ports cannot be determined reliably for any
// reason, or if there are no configured ports for the bridge.
//
// Example call: network.GetBridgePorts(network.SysClassNetPath, "br-eth1")
func GetBridgePorts(sysPath, bridgeName string) []string {
	portsGlobPath := filepath.Join(sysPath, bridgeName, "brif", "*")
	// Glob ignores I/O errors and can only return ErrBadPattern, which we treat
	// as no results, but for debugging we're still logging the error.
	paths, err := filepath.Glob(portsGlobPath)
	if err != nil {
		logger.Debugf("ignoring error traversing path %q: %v", portsGlobPath, err)
	}

	if len(paths) == 0 {
		return nil
	}

	// We need to convert full paths like /sys/class/net/br-eth0/brif/eth0 to
	// just names.
	names := make([]string, len(paths))
	for i := range paths {
		names[i] = filepath.Base(paths[i])
	}
	return names
}
