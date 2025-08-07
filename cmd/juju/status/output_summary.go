// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/juju/ansiterm"
	"github.com/juju/ansiterm/tabwriter"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/output"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/naturalsort"
)

func (c *statusCommand) formatSummary(writer io.Writer, value interface{}) error {

	if c.noColor {
		if _, ok := os.LookupEnv("NO_COLOR"); !ok {
			defer os.Unsetenv("NO_COLOR")
			os.Setenv("NO_COLOR", "")
		}

	}

	return FormatSummary(writer, c.color, value)
}

// FormatSummary writes a summary of the current environment
// including the following information:
// - Headers:
//   - All subnets the environment occupies.
//   - All ports the environment utilizes.
//
// - Sections:
//   - Machines: Displays total #, and then the # in each state.
//   - Units: Displays total #, and then # in each state.
//   - Applications: Displays total #, their names, and how many of each
//     are exposed.
//   - RemoteApplications: Displays total #, their names and URLs.
func FormatSummary(writer io.Writer, forceColor bool, value interface{}) error {
	fs, valueConverted := value.(formattedStatus)
	if !valueConverted {
		return errors.Errorf("expected value of type %T, got %T", fs, value)
	}

	f := newSummaryFormatter(writer, forceColor)
	stateToMachine := f.aggregateMachineStates(fs.Machines)
	appExposure := f.aggregateApplicationAndUnitStates(fs.Applications)
	kColor := ansiterm.Foreground(ansiterm.BrightCyan).SetStyle(ansiterm.Bold)
	k := f.delimitKeysWithTabs
	p := f.delimitValuesWithTabs

	// Print everything out
	k(kColor, "Running on subnets:")
	p(output.InfoHighlight, strings.Join(f.netStrings, ", "))
	k(kColor, " Utilizing ports:")
	printPorts(f.tw, strings.Split(f.portsInColumnsOf(3), ", "))
	f.tw.Println()
	f.tw.Flush()

	// Right align summary information
	f.tw.Init(writer, 0, 1, 2, ' ', tabwriter.AlignRight)
	if forceColor {
		f.tw.SetColorCapable(forceColor)
	}
	k(kColor, "# Machines:")
	p(output.EmphasisHighlight.Magenta, fmt.Sprintf("(%d)", len(fs.Machines)))
	f.printStateToCount(stateToMachine)
	p(output.CurrentHighlight, " ")

	k(kColor, "# Units:")
	p(output.EmphasisHighlight.Magenta, fmt.Sprintf("(%d)", f.numUnits))
	f.printStateToCount(f.stateToUnit)
	p(output.CurrentHighlight, " ")

	k(kColor, "# Applications:")
	p(output.EmphasisHighlight.Magenta, fmt.Sprintf("(%d)", len(fs.Applications)))
	for _, appName := range naturalsort.Sort(stringKeysFromMap(appExposure)) {
		s := appExposure[appName]
		k(output.EmphasisHighlight.BrightGreen, appName)
		p(output.CurrentHighlight, fmt.Sprintf("%d/%d\texposed", s[true], s[true]+s[false]))
	}
	p(output.CurrentHighlight, " ")

	k(kColor, "# Remote:")
	p(output.EmphasisHighlight.Magenta, fmt.Sprintf("(%d)", len(fs.RemoteApplications)))
	for _, svcName := range naturalsort.Sort(stringKeysFromMap(fs.RemoteApplications)) {
		s := fs.RemoteApplications[svcName]
		k(output.InfoHighlight, svcName)
		p(output.GoodHighlight, "", s.OfferURL)
	}
	f.tw.Flush()

	return nil
}

func newSummaryFormatter(writer io.Writer, forceColor bool) *summaryFormatter {
	w := output.TabWriter(writer)
	if forceColor {
		w.SetColorCapable(forceColor)
	}

	f := &summaryFormatter{
		ipAddrs:     make([]net.IPNet, 0),
		netStrings:  make([]string, 0),
		openPorts:   set.NewStrings(),
		stateToUnit: make(map[status.Status]int),
	}
	f.tw = &output.Wrapper{TabWriter: w}
	return f
}

type summaryFormatter struct {
	ipAddrs    []net.IPNet
	netStrings []string
	numUnits   int
	openPorts  set.Strings
	// status -> count
	stateToUnit map[status.Status]int
	tw          *output.Wrapper
}

func (f *summaryFormatter) delimitKeysWithTabs(ctx *ansiterm.Context, values ...string) {
	cCtx := output.EmphasisHighlight.BoldBrightMagenta
	for i, v := range values {
		if strings.Contains(v, ":") {
			splitted := strings.Split(v, ":")
			str := splitted[i]
			if ctx != nil {
				ctx.Fprintf(f.tw, "%s", str)
			} else {
				fmt.Fprintf(f.tw, "%s", str)
			}
			cCtx.Fprintf(f.tw, "%s\t", ":")
		} else {
			if ctx != nil {
				ctx.Fprintf(f.tw, "%s\t", v)
			} else {
				fmt.Fprintf(f.tw, "%s\t", v)
			}
		}
	}
	fmt.Fprint(f.tw)
}

func (f *summaryFormatter) delimitValuesWithTabs(ctx *ansiterm.Context, values ...string) {
	for _, v := range values {
		val := strings.TrimSpace(v)
		if strings.HasPrefix(val, "(") && strings.HasSuffix(val, ")") {
			val = strings.Split(strings.Split(val, "(")[1], ")")[0]
			fmt.Fprint(f.tw, "(")
			if ctx != nil {
				ctx.Fprintf(f.tw, "%s", val)
			} else {
				fmt.Fprintf(f.tw, "%s", val)
			}
			fmt.Fprint(f.tw, ")\t")
		} else {
			if ctx != nil {
				ctx.Fprintf(f.tw, "%s\t", v)
			} else {
				fmt.Fprintf(f.tw, "%s\t", v)
			}
		}
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
	f.stateToUnit[status.WorkloadStatusInfo.Current]++
}

func (f *summaryFormatter) printStateToCount(m map[status.Status]int) {
	for _, stateToCount := range naturalsort.Sort(stringKeysFromMap(m)) {
		numInStatus := m[status.Status(stateToCount)]
		f.delimitKeysWithTabs(output.StatusColor(status.Status(stateToCount)), stateToCount+":")
		f.delimitValuesWithTabs(output.EmphasisHighlight.Magenta, fmt.Sprintf(" %d ", numInStatus))
	}
}

func (f *summaryFormatter) trackIp(ip net.IP) {
	for _, net := range f.ipAddrs {
		if net.Contains(ip) {
			return
		}
	}

	ipNet := net.IPNet{IP: ip, Mask: ip.DefaultMask()}
	f.ipAddrs = append(f.ipAddrs, ipNet)
	f.netStrings = append(f.netStrings, ipNet.String())
}

func (f *summaryFormatter) resolveAndTrackIp(publicDns string) {
	// TODO(katco-): We may be able to utilize upcoming work which will expose these addresses outright.
	ip, err := net.ResolveIPAddr("ip4", publicDns)
	if err != nil {
		logger.Warningf(context.TODO(),
			"unable to resolve %s to an IP address. Status may be incorrect: %v",
			publicDns,
			err,
		)
		return
	}
	f.trackIp(ip.IP)
}

func (f *summaryFormatter) aggregateMachineStates(machines map[string]machineStatus) map[status.Status]int {
	stateToMachine := make(map[status.Status]int)
	for _, name := range naturalsort.Sort(stringKeysFromMap(machines)) {
		m := machines[name]
		f.resolveAndTrackIp(m.DNSName)

		machineStatus, _ := getStatusAndMessageFromMachineStatus(m)
		if machineStatus == "" {
			machineStatus = status.Pending
		}
		stateToMachine[machineStatus]++

	}
	return stateToMachine
}

func (f *summaryFormatter) aggregateApplicationAndUnitStates(applications map[string]applicationStatus) map[string]map[bool]int {
	svcExposure := make(map[string]map[bool]int)
	for _, name := range naturalsort.Sort(stringKeysFromMap(applications)) {
		s := applications[name]
		// Grab unit states
		for _, un := range naturalsort.Sort(stringKeysFromMap(s.Units)) {
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
