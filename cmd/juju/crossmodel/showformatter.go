// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/cmd/output"
)

const (
	// To wrap long lines within a column.
	maxColumnLength = 180
	truncatedSuffix = "..."
	maxFieldLength  = maxColumnLength - len(truncatedSuffix)
	columnWidth     = 45
)

// formatShowTabular returns a tabular summary of remote applications or
// errors out if parameter is not of expected type.
func formatShowTabular(writer io.Writer, value interface{}) error {
	endpoints, ok := value.(map[string]ShowRemoteApplication)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", endpoints, value)
	}
	return formatOfferedEndpointsTabular(writer, endpoints)
}

// formatOfferedEndpointsTabular returns a tabular summary of offered applications' endpoints.
func formatOfferedEndpointsTabular(writer io.Writer, all map[string]ShowRemoteApplication) error {
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}

	w.Println("Application URL", "Description", "Endpoint", "Interface", "Role")

	for name, one := range all {
		applicationName := name
		applicationDesc := one.Description

		// truncate long description for now.
		if len(applicationDesc) > maxColumnLength {
			applicationDesc = fmt.Sprintf("%v%v", applicationDesc[:maxFieldLength], truncatedSuffix)
		}
		descLines := breakLines(applicationDesc)

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
			w.Println(applicationName, descLine, name, endpoint.Interface, endpoint.Role)
			// Only print once.
			applicationName = ""
		}
	}
	tw.Flush()
	return nil
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
			lines[index] = aWord
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
