// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

const (
	// To wrap long lines within a column.
	maxColumnLength = 100
	truncatedSuffix = "..."
	maxFieldLength  = maxColumnLength - len(truncatedSuffix)
	columnWidth     = 30

	// To format things into columns.
	minwidth = 0
	tabwidth = 1
	padding  = 2
	padchar  = ' '
	flags    = 0
)

// formatShowTabular returns a tabular summary of remote services or
// errors out if parameter is not of expected type.
func formatShowTabular(value interface{}) ([]byte, error) {
	endpoints, ok := value.(map[string]ShowRemoteService)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", endpoints, value)
	}
	return formatOfferedEndpointsTabular(endpoints)
}

// formatOfferedEndpointsTabular returns a tabular summary of offered services' endpoints.
func formatOfferedEndpointsTabular(all map[string]ShowRemoteService) ([]byte, error) {
	var out bytes.Buffer
	const ()
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	print := func(values ...string) {
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}

	print("SERVICE", "DESCRIPTION", "ENDPOINT", "INTERFACE", "ROLE")

	for name, one := range all {
		serviceName := name
		serviceDesc := one.Description

		// truncate long description for now.
		if len(serviceDesc) > maxColumnLength {
			serviceDesc = fmt.Sprintf("%v%v", serviceDesc[:maxFieldLength], truncatedSuffix)
		}
		descLines := breakLines(serviceDesc)

		// Find the maximum amount of iterations required:
		// it will be either endpoints or description lines length
		maxIterations := max(len(one.Endpoints), len(descLines))

		names := []string{}
		for name, _ := range one.Endpoints {
			names = append(names, name)
		}
		sort.Strings(names)

		for i := 0; i < maxIterations; i++ {
			descLine := descAt(descLines, i)
			name, endpoint := endpointAt(one.Endpoints, names, i)
			print(serviceName, descLine, name, endpoint.Interface, endpoint.Role)
			// Only print once.
			serviceName = ""
		}
	}
	tw.Flush()

	return out.Bytes(), nil
}

func descAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

func endpointAt(endpoints map[string]RemoteEndpoint, names []string, i int) (string, RemoteEndpoint) {
	if i < len(endpoints) {
		name := names[i]
		return name, endpoints[name]
	}
	return "", RemoteEndpoint{}
}

func breakLines(text string) []string {
	words := strings.Fields(text)

	// if there is one very long word, break it
	if len(words) == 1 {
		return breakOneWord(words[0])
	}

	numLines := len(text)/columnWidth + 1
	lines := make([]string, numLines)

	index := 0
	for _, aWord := range words {
		if len(lines[index]) == 0 {
			lines[index] = aWord
			continue
		}
		tp := fmt.Sprintf("%v %v", lines[index], aWord)
		if len(tp) > columnWidth {
			index++
			continue
		}
		lines[index] = tp
	}

	return lines
}

func breakOneWord(one string) []string {
	if len(one) <= columnWidth {
		return []string{one}
	}

	numParts := (len(one) / columnWidth) + 1
	parts := make([]string, numParts)

	for i := 0; i < numParts; i++ {
		start := i * columnWidth
		end := start + columnWidth
		if end > len(one) {
			parts[i] = one[start:]
			continue
		}
		parts[i] = one[start:end]
	}
	return parts
}

func max(one, two int) int {
	if one > two {
		return one
	}
	return two
}
