// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/cmd/output"
)

// formatPoolListTabular returns a tabular summary of pool instances or
// errors out if parameter is not a map of PoolInfo.
func formatPoolListTabular(writer io.Writer, value interface{}) error {
	pools, ok := value.(map[string]PoolInfo)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", pools, value)
	}
	formatPoolsTabular(writer, pools)
	return nil
}

// formatPoolsTabular returns a tabular summary of pool instances.
func formatPoolsTabular(writer io.Writer, pools map[string]PoolInfo) {
	tw := output.TabWriter(writer)
	print := func(values ...string) {
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}

	print("NAME", "PROVIDER", "ATTRS")

	poolNames := make([]string, 0, len(pools))
	for name := range pools {
		poolNames = append(poolNames, name)
	}
	sort.Strings(poolNames)
	for _, name := range poolNames {
		pool := pools[name]
		// order by key for deterministic return
		keys := make([]string, 0, len(pool.Attrs))
		for key := range pool.Attrs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		attrs := make([]string, len(pool.Attrs))
		for i, key := range keys {
			attrs[i] = fmt.Sprintf("%v=%v", key, pool.Attrs[key])
		}
		print(name, pool.Provider, strings.Join(attrs, " "))
	}
	tw.Flush()
}
