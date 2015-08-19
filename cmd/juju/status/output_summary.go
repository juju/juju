// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
)

// FormatSummary returns a summary of the current environment
// including the following information:
// - Headers:
//   - All subnets the environment occupies.
//   - All ports the environment utilizes.
// - Sections:
//   - Machines: Displays total #, and then the # in each state.
//   - Units: Displays total #, and then # in each state.
//   - Services: Displays total #, their names, and how many of each
//     are exposed.
func FormatSummary(value interface{}) ([]byte, error) {
	fs, valueConverted := value.(formattedStatus)
	if !valueConverted {
		return nil, errors.Errorf("expected value of type %T, got %T", fs, value)
	}

	f := newSummaryFormatter()
	stateToMachine := f.aggregateMachineStates(fs.Machines)
	svcExposure := f.aggregateServiceAndUnitStates(fs.Services)
	p := f.delimitValuesWithTabs

	// Print everything out
	p("Running on subnets:", strings.Join(f.netStrings, ", "))
	p("Utilizing ports:", f.portsInColumnsOf(3))
	f.tw.Flush()

	// Right align summary information
	f.tw.Init(&f.out, 0, 2, 1, ' ', tabwriter.AlignRight)
	p("# MACHINES:", fmt.Sprintf("(%d)", len(fs.Machines)))
	f.printStateToCount(stateToMachine)
	p(" ")

	p("# UNITS:", fmt.Sprintf("(%d)", f.numUnits))
	f.printStateToCount(f.stateToUnit)
	p(" ")

	p("# SERVICES:", fmt.Sprintf(" (%d)", len(fs.Services)))
	for _, svcName := range common.SortStringsNaturally(stringKeysFromMap(svcExposure)) {
		s := svcExposure[svcName]
		p(svcName, fmt.Sprintf("%d/%d\texposed", s[true], s[true]+s[false]))
	}
	f.tw.Flush()

	return f.out.Bytes(), nil
}

func newSummaryFormatter() *summaryFormatter {
	f := &summaryFormatter{
		ipAddrs:     make([]net.IPNet, 0),
		netStrings:  make([]string, 0),
		openPorts:   set.NewStrings(),
		stateToUnit: make(map[params.Status]int),
	}
	f.tw = tabwriter.NewWriter(&f.out, 0, 1, 1, ' ', 0)
	return f
}

type summaryFormatter struct {
	ipAddrs    []net.IPNet
	netStrings []string
	numUnits   int
	openPorts  set.Strings
	// status -> count
	stateToUnit map[params.Status]int
	tw          *tabwriter.Writer
	out         bytes.Buffer
}

func (f *summaryFormatter) delimitValuesWithTabs(values ...string) {
	for _, v := range values {
		fmt.Fprintf(f.tw, "%s\t", v)
	}
	fmt.Fprintln(f.tw)
}

func (f *summaryFormatter) portsInColumnsOf(col int) string {

	var b bytes.Buffer
	for i, p := range f.openPorts.SortedValues() {
		if i != 0 && i%col == 0 {
			fmt.Fprintf(&b, "\n\t")
		}
		fmt.Fprintf(&b, "%s, ", p)
	}
	// Elide the last delimiter
	portList := b.String()
	if len(portList) >= 2 {
		return portList[:b.Len()-2]
	}
	return portList
}

func (f *summaryFormatter) trackUnit(name string, status unitStatus, indentLevel int) {
	f.resolveAndTrackIp(status.PublicAddress)

	for _, p := range status.OpenedPorts {
		if p != "" {
			f.openPorts.Add(p)
		}
	}
	f.numUnits++
	f.stateToUnit[status.AgentState]++
}

func (f *summaryFormatter) printStateToCount(m map[params.Status]int) {
	for _, status := range common.SortStringsNaturally(stringKeysFromMap(m)) {
		numInStatus := m[params.Status(status)]
		f.delimitValuesWithTabs(status+":", fmt.Sprintf(" %d ", numInStatus))
	}
}

func (f *summaryFormatter) trackIp(ip net.IP) {
	for _, net := range f.ipAddrs {
		if net.Contains(ip) {
			return
		}
	}

	ipNet := net.IPNet{ip, ip.DefaultMask()}
	f.ipAddrs = append(f.ipAddrs, ipNet)
	f.netStrings = append(f.netStrings, ipNet.String())
}

func (f *summaryFormatter) resolveAndTrackIp(publicDns string) {
	// TODO(katco-): We may be able to utilize upcoming work which will expose these addresses outright.
	ip, err := net.ResolveIPAddr("ip4", publicDns)
	if err != nil {
		logger.Warningf(
			"unable to resolve %s to an IP address. Status may be incorrect: %v",
			publicDns,
			err,
		)
		return
	}
	f.trackIp(ip.IP)
}

func (f *summaryFormatter) aggregateMachineStates(machines map[string]machineStatus) map[params.Status]int {
	stateToMachine := make(map[params.Status]int)
	for _, name := range common.SortStringsNaturally(stringKeysFromMap(machines)) {
		m := machines[name]
		f.resolveAndTrackIp(m.DNSName)

		if agentState := m.AgentState; agentState == "" {
			agentState = params.StatusPending
		} else {
			stateToMachine[agentState]++
		}
	}
	return stateToMachine
}

func (f *summaryFormatter) aggregateServiceAndUnitStates(services map[string]serviceStatus) map[string]map[bool]int {
	svcExposure := make(map[string]map[bool]int)
	for _, name := range common.SortStringsNaturally(stringKeysFromMap(services)) {
		s := services[name]
		// Grab unit states
		for _, un := range common.SortStringsNaturally(stringKeysFromMap(s.Units)) {
			u := s.Units[un]
			f.trackUnit(un, u, 0)
			recurseUnits(u, 1, f.trackUnit)
		}

		if _, ok := svcExposure[name]; !ok {
			svcExposure[name] = make(map[bool]int)
		}

		svcExposure[name][s.Exposed]++
	}
	return svcExposure
}
