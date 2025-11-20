// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	apiclient "github.com/juju/juju/api/client/client"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/output"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/juju/osenv"
)

// TODO(peritto666) - add tests

// NewStatusHistoryCommand returns a command that reports the history
// of status changes for the specified unit.
func NewStatusHistoryCommand() cmd.Command {
	statusCmd := &statusHistoryCommand{}
	statusCmd.getStatusHistoryCollectors = getStatusHistoryCollectors(statusCmd)
	return modelcmd.Wrap(statusCmd)
}

// HistoryAPI is the API surface for the show-status-log command.
type HistoryAPI interface {
	StatusHistory(ctx context.Context, kind status.HistoryKind, tag names.Tag, filter status.StatusHistoryFilter) (status.History, error)
	Close() error
}

type statusHistoryCommand struct {
	modelcmd.ModelCommandBase
	out             cmd.Output
	backlogSize     int
	backlogSizeDays int
	isoTime         bool

	// fetched from flags but not used as is
	rawKind string
	rawDate string

	// Resolved from flags or params in Init
	date       time.Time
	entityName string
	kind       status.HistoryKind

	// This function is injected for testing purposes
	getStatusHistoryCollectors func() ([]historyCollector, error)
}

var statusHistoryDoc = fmt.Sprintf(`
This command will report the history of status changes for
a given entity.

The statuses are available for the following types.
-type supports:
%v
 and sorted by time of occurrence.

 The default is unit.
`, supportedHistoryKindDescs())

const statusHistoryExamples = `
Show the status history for the specified unit:

    juju show-status-log mysql/0

Show the status history for the specified unit with the last 30 logs:

    juju show-status-log mysql/0 -n 30

Show the status history for the specified unit with the logs for the past 2 days:

    juju show-status-log mysql/0 -days 2

Show the status history for the specified unit with the logs for any date after 2020-01-01:

    juju show-status-log mysql/0 --from-date 2020-01-01

Show the status history for the specified application:

    juju show-status-log -type application wordpress

Show the status history for the specified machine:

    juju show-status-log 0

Show the status history for the model:

    juju show-status-log -type model
`

func (c *statusHistoryCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "show-status-log",
		Args:     "<entity name>",
		Purpose:  "Output past statuses for the specified entity.",
		Doc:      statusHistoryDoc,
		Examples: statusHistoryExamples,
		SeeAlso: []string{
			"status",
		},
	})
}

func supportedHistoryKindTypes() string {
	supported := set.NewStrings()
	for k := range status.AllHistoryKind() {
		supported.Add(string(k))
	}
	return strings.Join(supported.SortedValues(), "|")
}

func supportedHistoryKindDescs() string {
	types := status.AllHistoryKind()
	supported := set.NewStrings()
	for k := range types {
		supported.Add(string(k))
	}
	all := ""
	for _, k := range supported.SortedValues() {
		all += fmt.Sprintf("    %v:  %v\n", k, types[status.HistoryKind(k)])
	}
	return all
}

func (c *statusHistoryCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.rawKind, "type", "unit", fmt.Sprintf("Type of statuses to be displayed [%v]", supportedHistoryKindTypes()))
	f.IntVar(&c.backlogSize, "n", 0, "Returns the last N logs (cannot be combined with --days or --date)")
	f.IntVar(&c.backlogSizeDays, "days", 0, "Returns the logs for the past <days> days (cannot be combined with -n or --date)")
	f.StringVar(&c.rawDate, "from-date", "", "Returns logs for any date after the passed one, the expected date format is YYYY-MM-DD (cannot be combined with -n or --days)")
	f.BoolVar(&c.isoTime, "utc", false, "Display time as UTC in RFC3339 format")

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatTabular,
	})
}

func (c *statusHistoryCommand) Init(args []string) error {
	switch {
	case len(args) > 1:
		return errors.Errorf("unexpected arguments after entity name.")
	case len(args) == 0:
		if c.rawKind != status.KindModel.String() {
			return errors.Errorf("entity name is missing.")
		}
	default:
		c.entityName = args[0]
	}
	// If use of ISO time not specified on command line,
	// check env var.
	if !c.isoTime {
		var err error
		envVarValue := os.Getenv(osenv.JujuStatusIsoTimeEnvKey)
		if envVarValue != "" {
			if c.isoTime, err = strconv.ParseBool(envVarValue); err != nil {
				return errors.Errorf("invalid %s env var, expected true|false: %w", osenv.JujuStatusIsoTimeEnvKey, err)
			}
		}
	}
	emptyDate := c.rawDate == ""
	emptySize := c.backlogSize == 0
	emptyDays := c.backlogSizeDays == 0
	if emptyDate && emptySize && emptyDays {
		c.backlogSize = 20
	}
	if (!emptyDays && !emptySize) || (!emptyDays && !emptyDate) || (!emptySize && !emptyDate) {
		return errors.Errorf("backlog size, backlog date and backlog days back cannot be specified together")
	}
	if c.rawDate != "" {
		var err error
		c.date, err = time.Parse("2006-01-02", c.rawDate)
		if err != nil {
			return errors.Errorf("parsing backlog date: %w", err)
		}
	}

	c.kind = status.HistoryKind(c.rawKind)
	if !c.kind.Valid() {
		return errors.Errorf("unexpected status type %q", c.rawKind)
	}
	return nil
}

// DetailedStatus holds status info about a machine or unit agent.
type DetailedStatus struct {
	Status  status.Status          `yaml:"status,omitempty" json:"status,omitempty"`
	Message string                 `yaml:"message,omitempty" json:"message,omitempty"`
	Data    map[string]interface{} `yaml:"data,omitempty" json:"data,omitempty"`
	Since   *time.Time             `yaml:"since,omitempty" json:"since,omitempty"`
	Kind    status.HistoryKind     `yaml:"type,omitempty" json:"type,omitempty"`
}

// History holds the status results.
type History []DetailedStatus

func (c *statusHistoryCommand) Run(ctx *cmd.Context) error {
	filterArgs := c.getFilterArgs()
	tag, err := c.getEntityTag(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	statuses, err := c.fetchStatusHistory(ctx, tag, filterArgs)
	historyLen := len(statuses)
	if err != nil {
		if historyLen == 0 {
			return errors.Capture(err)
		}
		// Display any error, but continue to print status if some was returned
		fmt.Fprintf(ctx.Stderr, "%v\n", err)
	}

	if historyLen == 0 {
		return errors.Errorf("no status history available")
	}
	history := make(History, len(statuses))
	for i, h := range statuses {
		history[i] = DetailedStatus{
			Status:  h.Status,
			Message: h.Info,
			Data:    h.Data,
			Since:   h.Since,
			Kind:    h.Kind,
		}
	}
	return c.out.Write(ctx, history)
}

// getFilterArgs returns the filter args for the status history command.
func (c *statusHistoryCommand) getFilterArgs() status.StatusHistoryFilter {
	var delta *time.Duration

	if c.backlogSizeDays != 0 {
		t := time.Duration(c.backlogSizeDays*24) * time.Hour
		delta = &t
	}
	filterArgs := status.StatusHistoryFilter{
		Size:  c.backlogSize,
		Delta: delta,
	}

	if !c.date.IsZero() {
		filterArgs.FromDate = &c.date
	}
	return filterArgs
}

// getEntityTag returns the tag for the entity specified by the command line args.
func (c *statusHistoryCommand) getEntityTag(ctx *cmd.Context) (names.Tag, error) {
	switch c.kind {
	case status.KindModel:
		_, details, err := c.ModelDetails(ctx)
		if err != nil {
			return nil, errors.Capture(err)
		}
		return names.NewModelTag(details.ModelUUID), nil
	case status.KindUnit, status.KindWorkload, status.KindUnitAgent:
		if !names.IsValidUnit(c.entityName) {
			return nil, errors.Errorf("%q is not a valid name for a %s", c.entityName, c.kind)
		}
		return names.NewUnitTag(c.entityName), nil
	case status.KindApplication, status.KindSAAS:
		if !names.IsValidApplication(c.entityName) {
			return nil, errors.Errorf("%q is not a valid name for an application", c.entityName)
		}
		return names.NewApplicationTag(c.entityName), nil
	default:
		if !names.IsValidMachine(c.entityName) {
			return nil, errors.Errorf("%q is not a valid name for a %s", c.entityName, c.kind)
		}
		return names.NewMachineTag(c.entityName), nil
	}
}

func (c *statusHistoryCommand) formatTabular(writer io.Writer, value interface{}) error {
	h, ok := value.(History)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", History{}, value)
	}
	c.writeTabular(writer, h)
	return nil
}

func (c *statusHistoryCommand) writeTabular(writer io.Writer, statuses History) {
	tw := output.TabWriter(writer)
	w := output.Wrapper{TabWriter: tw}

	w.Println("Time", "Type", "Status", "Message")
	for _, v := range statuses {
		w.Print(common.FormatTime(v.Since, c.isoTime), v.Kind)
		w.PrintStatus(v.Status)
		w.Println(v.Message)
	}
	tw.Flush()
}

// fetchStatusHistory fetches the status history for the specified entity.
func (c *statusHistoryCommand) fetchStatusHistory(ctx *cmd.Context,
	tag names.Tag, args status.StatusHistoryFilter) (status.History, error) {
	collectors, err := c.getStatusHistoryCollectors()
	if err != nil {
		return nil, errors.Capture(err)
	}

	results := make(chan collectorResult, len(collectors))

	var wg sync.WaitGroup
	for _, collector := range collectors {
		wg.Add(1)
		go func(collector historyCollector) {
			results <- collector(ctx, c.kind, tag, args)
			wg.Done()
		}(collector)
	}
	wg.Wait()
	close(results)

	var combined status.History
	var errs []error
	for result := range results {
		if result.err != nil {
			errs = append(errs, result.err)
		}
		combined = append(combined, result.history...)
	}

	// Deduplicate combined history to avoid duplicated entries coming from
	// multiple controllers. Use a composite key of fields that define a
	// history entry. We include kind, status, message (info), and the
	// timestamp with nanosecond precision. We intentionally exclude the Data
	// map from the key to avoid non-determinism from map iteration order.
	seen := make(map[string]struct{})
	dedup := make(status.History, 0, len(combined))
	for _, h := range combined {
		// Build a deterministic key. Handle nil time safely.
		since := "<nil>"
		if h.Since != nil {
			// Use UTC to normalise across controllers/timezones.
			since = h.Since.UTC().Format(time.RFC3339Nano)
		}
		key := fmt.Sprintf("%s|%s|%s|%s", h.Kind, since, h.Status, h.Info)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		dedup = append(dedup, h)
	}

	// Sort the oldest first to align with the presentation and to make size capping
	// deterministic when limiting results.
	sort.Slice(dedup, func(i, j int) bool {
		if dedup[i].Since == nil {
			return true
		}
		if dedup[j].Since == nil {
			return false
		}
		return dedup[i].Since.Before(*dedup[j].Since)
	})

	// If a size was requested, ensure we don't return more entries than
	// expected overall (across all controllers).
	if args.Size > 0 && len(dedup) > args.Size {
		dedup = dedup[len(dedup)-args.Size:]
	}

	return dedup, errors.Join(errs...)
}

// historyCollector defines a function type for retrieving status history data
// based on provided parameters.
type historyCollector func(ctx context.Context, kind status.HistoryKind, tag names.Tag,
	filter status.StatusHistoryFilter) collectorResult

// collectorResult defines the result of a historyCollector invocation.
type collectorResult struct {
	history status.History
	err     error
}

// getStatusHistoryCollectors returns a function that retrieves a list of
// history collectors for the controller.
// It abstracts away the logic of retrieving the controller details and
// initializing collectors for each controller API endpoint, in case of HA
// setting.
func getStatusHistoryCollectors(c *statusHistoryCommand) func() ([]historyCollector, error) {
	return func() ([]historyCollector, error) {
		controllerName, err := c.ModelCommandBase.ControllerName()
		if err != nil {
			return nil, errors.Errorf("getting controller name: %v", err)
		}
		details, err := c.ClientStore().ControllerByName(controllerName)
		if err != nil {
			return nil, errors.Errorf("getting controller details: %v", err)
		}

		if len(details.APIEndpoints) == 0 {
			return nil, errors.Errorf("no controllers found")
		}

		var collectors []historyCollector
		for _, addr := range details.APIEndpoints {
			collectors = append(collectors, c.newHistoryCollector(addr))
		}

		return collectors, nil
	}
}

// newHistoryCollector creates a historyCollector function that retrieves status
// history data from a specified API address. It ensures that the connection
// is closed once the data is retrieved.
func (c *statusHistoryCommand) newHistoryCollector(addr string) historyCollector {
	return func(ctx context.Context, kind status.HistoryKind, tag names.Tag, filter status.StatusHistoryFilter) collectorResult {
		root, err := c.NewAPIRootWithAddressOverride(ctx, []string{addr})
		if err != nil {
			return collectorResult{history: nil, err: errors.Capture(err)}
		}
		client := apiclient.NewClient(root, logger)
		defer func() {
			_ = client.Close()
		}()

		history, err := client.StatusHistory(ctx, kind, tag, filter)

		return collectorResult{history: history, err: errors.Capture(err)}
	}
}
