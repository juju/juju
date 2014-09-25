package main

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

// FormatOneline returns a brief list of units and their subordinates.
// Subordinates will be indented 2 spaces and listed under their
// superiors.
func FormatOneline(value interface{}) ([]byte, error) {
	fs, valueConverted := value.(formattedStatus)
	if !valueConverted {
		return nil, fmt.Errorf("could not convert the incoming value to type formattedStatus.")
	}
	var out bytes.Buffer

	pprint := func(uName string, u unitStatus, level int) {
		fmt.Fprintf(&out, indent("\n", level*2, "- %s: %s (%v)"), uName, u.PublicAddress, u.AgentState)
	}

	for _, svcName := range sortStrings(stringKeysFromMap(fs.Services)) {
		svc := fs.Services[svcName]
		for _, uName := range sortStrings(stringKeysFromMap(svc.Units)) {
			unit := svc.Units[uName]
			pprint(uName, unit, 0)
			recurseUnits(unit, 1, pprint)
		}
	}

	return out.Bytes(), nil
}

// FormatTabular returns a tabular summary of machines, services, and
// units. Any subordinate items are indented by two spaces beneath
// their superior.
func FormatTabular(value interface{}) ([]byte, error) {
	fs, ok := value.(formattedStatus)
	if !ok {
		return nil, errors.Errorf("could not convert the incoming value to type formattedStatus.")
	}
	var out bytes.Buffer
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)
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
	for _, svcName := range sortStrings(stringKeysFromMap(fs.Services)) {
		svc := fs.Services[svcName]
		for un, u := range svc.Units {
			units[un] = u
		}
		p(svcName, fmt.Sprintf("%t", svc.Exposed), svc.Charm)
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
		const indentationLevel = 1
		recurseUnits(u, indentationLevel, pUnit)
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
	return fmt.Sprintf("%s%*s%s", prepend, level, "", append)
}
