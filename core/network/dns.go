// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"strings"

	"github.com/juju/juju/internal/errors"
)

// DNSConfig holds a list of DNS nameserver addresses
// and default search domains.
type DNSConfig struct {
	Nameservers   []string
	SearchDomains []string
}

// ParseResolvConf parses a resolv.conf(5) file at the given path (usually
// "/etc/resolv.conf"), if present. Returns the values of any 'nameserver'
// stanzas, and the last 'search' stanza found. Values in the result will
// appear in the order found, including duplicates.
// Parsing errors will be returned in these cases:
//
//  1. if a 'nameserver' or 'search' without a value is found;
//  2. 'nameserver' with more than one value (trailing comments starting with
//     '#' or ';' after the value are allowed).
//  3. if any value containing '#' or ';' (e.g. 'nameserver 8.8.8.8#bad'),
//     because values and comments following them must be separated by
//     whitespace.
//
// No error is returned if the file is missing.
// See resolv.conf(5) man page for details.
func ParseResolvConf(path string) (*DNSConfig, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		logger.Debugf(context.TODO(), "%q does not exist - not parsing", path)
		return nil, nil
	} else if err != nil {
		return nil, errors.Capture(err)
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
			return nil, errors.Errorf("parsing %q, line %d: %w", path, lineNum, err)
		}

		if numValues := len(values); numValues > 1 {
			return nil, errors.Errorf(
				"parsing %q, line %d: one value expected for \"nameserver\", got %d",
				path, lineNum, numValues)

		} else if numValues == 1 {
			nameservers = append(nameservers, values[0])
			continue
		}

		values, err = parseResolvStanza(line, "search")
		if err != nil {
			return nil, errors.Errorf("parsing %q, line %d: %w", path, lineNum, err)
		}

		if len(values) > 0 {
			// Last 'search' found wins.
			searchDomains = values
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, errors.Errorf("reading %q: %w", path, err)
	}

	return &DNSConfig{
		Nameservers:   nameservers,
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
