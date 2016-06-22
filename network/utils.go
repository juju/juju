// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"bufio"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

// DNSConfig holds a list of DNS nameserver addresses and default search
// domains.
type DNSConfig struct {
	Nameservers   []Address
	SearchDomains []string
}

// ParseResolvConf parses the given resolvConfPath (usually "/etc/resolv.conf")
// file (if present), extracting all 'nameserver' and 'search' stanzas, and
// returns them.
func ParseResolvConf(resolvConfPath string) (*DNSConfig, error) {
	file, err := os.Open(resolvConfPath)
	if os.IsNotExist(err) {
		logger.Debugf("%q does not exist, not parsing", resolvConfPath)
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	defer file.Close()

	nameservers := set.NewStrings()
	searchDomains := set.NewStrings()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if nameserver, ok := parseResolvStanzaValue(line, "nameserver"); ok {
			nameservers.Add(nameserver)
		}

		if domain, ok := parseResolvStanzaValue(line, "search"); ok {
			searchDomains.Add(domain)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, errors.Annotatef(err, "cannot read DNS config from %q", resolvConfPath)
	}

	result := new(DNSConfig)
	if !nameservers.IsEmpty() {
		result.Nameservers = NewAddresses(nameservers.SortedValues()...)
	}
	if !searchDomains.IsEmpty() {
		result.SearchDomains = searchDomains.SortedValues()
	}

	return result, nil
}

func parseResolvStanzaValue(inputLine, stanza string) (value string, ok bool) {
	inputLine = strings.TrimSpace(inputLine)
	if strings.HasPrefix(inputLine, "#") {
		// Skip comments.
		return "", false
	}

	if strings.HasPrefix(inputLine, stanza) {
		value = strings.TrimPrefix(inputLine, stanza)
		// Drop comments after the value, if any.
		if strings.Contains(value, "#") {
			value = value[:strings.Index(value, "#")]
		}
		value = strings.TrimSpace(value)
		ok = true
	}

	return value, ok
}
