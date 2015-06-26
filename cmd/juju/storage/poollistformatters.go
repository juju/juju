package storage

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

// formatPoolListTabular returns a tabular summary of pool instances or
// errors out if parameter is not a map of PoolInfo.
func formatPoolListTabular(value interface{}) ([]byte, error) {
	pools, ok := value.(map[string]PoolInfo)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", pools, value)
	}
	return formatPoolsTabular(pools)
}

// formatPoolsTabular returns a tabular summary of pool instances.
func formatPoolsTabular(pools map[string]PoolInfo) ([]byte, error) {
	var out bytes.Buffer
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
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

	return out.Bytes(), nil
}
