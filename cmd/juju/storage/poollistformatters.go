package storage

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

// formatPoolListTabular returns a tabular summary of pool instances.
func formatPoolListTabular(value interface{}) ([]byte, error) {
	pools, ok := value.(map[string]PoolInfo)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", pools, value)
	}
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
	p := func(values ...interface{}) {
		for _, v := range values {
			fmt.Fprintf(tw, "%v\t", v)
		}
		fmt.Fprintln(tw)
	}

	p("NAME\tPROVIDER\tATTRS")

	poolNames := make([]string, 0, len(pools))
	for name := range pools {
		poolNames = append(poolNames, name)
	}
	sort.Strings(poolNames)
	for _, name := range poolNames {
		pool := pools[name]
		traits := make([]string, len(pool.Attrs))
		var i int
		for key, value := range pool.Attrs {
			traits[i] = fmt.Sprintf("%v=%v", key, value)
			i++
		}
		p(name, pool.Provider, strings.Join(traits, ","))
	}
	tw.Flush()

	return out.Bytes(), nil
}
