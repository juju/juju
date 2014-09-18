package main

import (
	"bytes"
	"fmt"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"net"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
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
func FormatSummary(value interface{}) ([]byte, error) {
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

	// Track all IP Addresses we're utilizing.
	var ipAddrs []net.IPNet
	var netStrings []string
	trackIp := func(ip net.IP) {
		for _, net := range ipAddrs {
			if net.Contains(ip) {
				return
			}
		}

		ipNet := net.IPNet{ip, ip.DefaultMask()}
		ipAddrs = append(ipAddrs, ipNet)
		netStrings = append(netStrings, ipNet.String())
	}
	resolveAndTrackIp := func(publicDns string) error {
		if ip, err := net.ResolveIPAddr("ip4", publicDns); err != nil {
			return errors.Annotate(err, "unable to determine IP of host.")
		} else {
			trackIp(ip.IP)
		}
		return nil
	}
	printStateToCount := func(m map[params.Status]int) {
		for _, status := range sortStrings(stringKeysFromMap(m)) {
			numInStatus := m[params.Status(status)]
			p(status+":", fmt.Sprintf(" %s ", strconv.Itoa(numInStatus)))
		}
	}
	logIPResolutionWarning := func(address string, err error) {
		logger.Warningf(
			"unable to resolve %s to an IP address. Status may be incorrect: %v",
			address,
			err,
		)
	}

	var openPorts []string
	stateToUnit := make(map[params.Status]int)
	numUnits := 0
	trackUnit := func(name string, status unitStatus, indentLevel int) {
		if err := resolveAndTrackIp(status.PublicAddress); err != nil {
			logIPResolutionWarning(status.PublicAddress, err)
		}
		openPorts = append(openPorts, status.OpenedPorts...)
		numUnits += 1
		stateToUnit[status.AgentState] += 1
	}

	// Aggregate machine states.
	stateToMachine := make(map[params.Status]int)
	for _, name := range sortStrings(stringKeysFromMap(fs.Machines)) {
		m := fs.Machines[name]
		if err := resolveAndTrackIp(m.DNSName); err != nil {
			logIPResolutionWarning(m.DNSName, err)
		}

		if agentState := m.AgentState; agentState == "" {
			agentState = params.StatusPending
		} else {
			stateToMachine[agentState] += 1
		}
	}

	// Aggregate service & unit states.
	svcExposure := make(map[string]map[bool]int)
	for _, name := range sortStrings(stringKeysFromMap(fs.Services)) {
		s := fs.Services[name]
		// Grab unit states
		for _, un := range sortStrings(stringKeysFromMap(s.Units)) {
			u := s.Units[un]
			trackUnit(un, u, 0)
			recurseUnits(u, 1, trackUnit)
		}

		if _, ok := svcExposure[name]; !ok {
			svcExposure[name] = make(map[bool]int)
		}

		svcExposure[name][s.Exposed] += 1
	}

	// Print everything out
	p("Running on subnets:", strings.Join(netStrings, ", "))
	p("Utilizing ports:", strings.Join(openPorts, ", "), "\n")
	tw.Flush()

	// Right align summary information
	tw.Init(&out, 0, 2, 1, ' ', tabwriter.AlignRight)
	p("# MACHINES:", fmt.Sprintf("(%d)", len(fs.Machines)))
	printStateToCount(stateToMachine)
	p(" ")
	p("# UNITS:", fmt.Sprintf("(%d)", numUnits))
	printStateToCount(stateToUnit)
	p(" ")

	p("# SERVICES:", fmt.Sprintf(" (%d)", len(fs.Services)))

	for _, svcName := range sortStrings(stringKeysFromMap(svcExposure)) {
		s := svcExposure[svcName]
		p(svcName, fmt.Sprintf("%d/%d\texposed", s[true], s[true]+s[false]))
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
