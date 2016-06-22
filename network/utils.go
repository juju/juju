// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"bufio"
	"os"
	"strings"

	"github.com/juju/errors"
)

// DNSConfig holds a list of DNS nameserver addresses and default search
// domains.
type DNSConfig struct {
	Nameservers   []Address
	SearchDomains []string
}

// ParseResolvConf parses the given resolvConfPath (usually "/etc/resolv.conf")
// file (if present). It returns the values of any 'nameserver' stanzas, and the
// last 'search' stanza found, as defined by resolv.conf(5). Values in the
// result will appear in the order found, including duplicates. Parsing errors
// will be returned in these cases:
//
// 1. if a 'nameserver' or 'search' without a value is found;
// 2. 'nameserver' with more than one value (trailing comments starting with '#'
//    or ';' after the value are allowed).
// 3. if any value containing '#' or ';' (e.g. 'nameserver 8.8.8.8#bad'), because
//    values and comments following them must be separated by whitespace.
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
			searchDomains = append([]string(nil), values...)
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
