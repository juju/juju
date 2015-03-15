// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

// formatListTabular returns a tabular summary of storage instances.
func formatListTabular(value interface{}) ([]byte, error) {
	storageInfo, ok := value.(map[string]map[string]StorageInfo)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", storageInfo, value)
	}
	var out bytes.Buffer
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)
	p := func(values ...interface{}) {
		for _, v := range values {
			fmt.Fprintf(tw, "%v\t", v)
		}
		fmt.Fprintln(tw)
	}
	p("[Storage]")
	p("UNIT\tID\tLOCATION")

	// First sort by owners
	owners := make([]string, 0, len(storageInfo))
	for order := range storageInfo {
		owners = append(owners, order)
	}
	sort.Strings(owners)
	for _, owner := range owners {
		all := storageInfo[owner]

		// Then sort by storage ids
		storageIds := make([]string, 0, len(all))
		for anId := range all {
			storageIds = append(storageIds, anId)
		}
		sort.Strings(byStorageId(storageIds))

		for _, storageId := range storageIds {
			info := all[storageId]
			p(info.UnitId, storageId, info.Location)
		}
	}
	tw.Flush()

	return out.Bytes(), nil
}

type byStorageId []string

func (s byStorageId) Len() int {
	return len(s)
}

func (s byStorageId) Swap(a, b int) {
	s[a], s[b] = s[b], s[a]
}

func (s byStorageId) Less(a, b int) bool {
	sa := strings.SplitN(s[a], "/", 2)
	sb := strings.SplitN(s[b], "/", 2)
	if sa[0] < sb[0] {
		return true
	}
	return sa[0] == sb[0] && sa[1] < sb[1]
}
