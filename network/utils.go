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
// first 'search' stanza, as defined by resolv.conf(5). Values in the result
// will appear in the order found, including duplicates.
func ParseResolvConf(resolvConfPath string) (*DNSConfig, error) {
	file, err := os.Open(resolvConfPath)
	if os.IsNotExist(err) {
		logger.Debugf("%q does not exist, not parsing", resolvConfPath)
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
	for scanner.Scan() {
		line := scanner.Text()

		if values, ok := parseResolvStanzaValues(line, "nameserver"); ok {
			nameservers = append(nameservers, values[0]) // a single value is allowed only
		}

		if values, ok := parseResolvStanzaValues(line, "search"); ok && searchDomains == nil {
			searchDomains = append(searchDomains, values...)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, errors.Annotatef(err, "cannot read DNS config from %q", resolvConfPath)
	}

	result := new(DNSConfig)
	if len(nameservers) > 0 {
		result.Nameservers = NewAddresses(nameservers...)
	}
	if len(searchDomains) > 0 {
		result.SearchDomains = searchDomains
	}

	return result, nil
}

func parseResolvStanzaValues(inputLine, stanza string) (values []string, ok bool) {
	if strings.IndexAny(inputLine, ";#") == 0 {
		// ';' or '#', but only in the first column is considered a comment. See
		// resolv.conf(5).
		return nil, false
	}

	if strings.HasPrefix(inputLine, stanza) {
		// resolv.conf(5) states that to be recognized as valid, the line must
		// begin with the stanza, followed by whitespace, then the value(s),
		// also separated with whitespace.
		values = strings.Fields(inputLine)
		if len(values) >= 2 {
			values = values[1:] // skip the stanza
			ok = true
		}
	}

	return values, ok
}
