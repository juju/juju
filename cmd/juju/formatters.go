package main

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

// FormatTabular returns a tabular summary of machines, services, and
// units. Any subordinate items are indented by two spaces beneath
// their superior.
func FormatTabular(value interface{}) ([]byte, error) {
	fs, ok := value.(formattedStatus)
	if !ok {
		return nil, fmt.Errorf("could not convert the incoming value to type formattedStatus.")
	}
	out := new(bytes.Buffer)
	// To format things into columns.
	tw := tabwriter.NewWriter(out, 0, 1, 1, ' ', 0)
	p := func(values ...interface{}) {
		for _, v := range values {
			fmt.Fprintf(tw, "%s\t", v)
		}
		fmt.Fprintln(tw)
	}

	p("[Machines]")
	p("ID\tSTATE\tVERSION\tDNS\tINS-ID\tSERIES\tHARDWARE")
	for _, name := range sortStrings(stringKeysFromMap(fs.Machines)) {
		m := fs.Machines[name]
		p(m.Id, m.AgentState, m.AgentVersion, m.DNSName, m.InstanceId, m.Series, m.Hardware)
	}
	tw.Flush()

	units := make(map[string]unitStatus)

	p("\n[Services]")
	p("NAME\tEXPOSED\tCHARM")
	for _, name := range sortStrings(stringKeysFromMap(fs.Services)) {
		s := fs.Services[name]
		for un, u := range s.Units {
			if _, ok := units[un]; ok {
				panic("Doh")
			}
			units[un] = u
		}
		p(name, fmt.Sprintf("%t", s.Exposed), s.Charm)
	}
	tw.Flush()

	pUnit := func(name string, u unitStatus, level int) {
		p(
			indent("", level*2, name),
			u.AgentState,
			u.AgentVersion,
			u.Machine,
			strings.Join(u.OpenedPorts, ","),
			u.PublicAddress,
		)
	}

	p("\n[Units]")
	p("ID\tSTATE\tVERSION\tMACHINE\tPORTS\tPUBLIC-ADDRESS")
	for _, name := range sortStrings(stringKeysFromMap(units)) {
		u := units[name]
		pUnit(name, u, 0)
		recurseUnits(u, 1, pUnit)
	}
	tw.Flush()

	return out.Bytes(), nil
}

// sortStrings is syntactic sugar so we can do sorts in one line.
func sortStrings(s []string) []string {
	sort.Strings(s)
	return s
}

// stringKeysFromMap takes a map with keys which are strings and returns
// only the keys.
func stringKeysFromMap(m interface{}) (keys []string) {
	for _, k := range reflect.ValueOf(m).MapKeys() {
		keys = append(keys, k.String())
	}
	return
}

// recurseUnits calls the given recurseMap function on the given unit
// and its subordinates (recursively defined on the given unit).
func recurseUnits(u unitStatus, il int, recurseMap func(string, unitStatus, int)) {
	if len(u.Subordinates) == 0 {
		return
	}
	for _, uName := range sortStrings(stringKeysFromMap(u.Subordinates)) {
		unit := u.Subordinates[uName]
		recurseMap(uName, unit, il)
		recurseUnits(unit, il+1, recurseMap)
	}
}

// indent prepends a format string with the given number of spaces.
func indent(prepend string, level int, append string) string {
	return prepend + fmt.Sprintf("%"+strconv.Itoa(level)+"s", "") + append
}
