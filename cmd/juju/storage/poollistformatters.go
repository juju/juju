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
		fmt.Fprintf(tw, strings.Join(values, "\t"))
		// Output newline after each pool
		fmt.Fprintln(tw)
	}

	print("NAME", "PROVIDER", "ATTRS")

	poolNames := make([]string, 0, len(pools))
	for name := range pools {
		poolNames = append(poolNames, name)
	}
	sort.Strings(poolNames)
	for _, name := range poolNames {
		pool := pools[name]
		attrs := make([]string, len(pool.Attrs))
		var i int
		for key, value := range pool.Attrs {
			attrs[i] = fmt.Sprintf("%v=%v", key, value)
			i++
		}
		print(name, pool.Provider, strings.Join(attrs, " "))
	}
	tw.Flush()

	return out.Bytes(), nil
}
