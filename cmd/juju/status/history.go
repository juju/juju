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
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/highavailability"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/output"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/juju/osenv"
)

// TODO(peritto666) - add tests

// NewStatusHistoryCommand returns a command that reports the history
// of status changes for the specified unit.
func NewStatusHistoryCommand() cmd.Command {
	return modelcmd.Wrap(&statusHistoryCommand{})
}

// HistoryAPI is the API surface for the show-status-log command.
type HistoryAPI interface {
	// StatusHistory returns the status history for the given entity tag
	// and kind, filtered according to the provided filter.
	StatusHistory(ctx context.Context, kind status.HistoryKind, tag names.Tag, filter status.StatusHistoryFilter) (status.History, error)

	// Close closes the API client.
	Close() error
}

type statusHistoryCommand struct {
	modelcmd.ModelCommandBase
	clients         []HistoryAPI
	out             cmd.Output
	outputContent   string
	backlogSize     int
	backlogSizeDays int
	backlogDate     string
	isoTime         bool
	entityName      string
	date            time.Time
}

var statusHistoryDoc = fmt.Sprintf(`
This command will report the history of status changes for
a given entity.

The statuses are available for the following types.
--type supports:
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

    juju show-status-log --type application wordpress

Show the status history for the specified machine:

    juju show-status-log 0

Show the status history for the model:

    juju show-status-log --type model
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
	var all strings.Builder
	for _, k := range supported.SortedValues() {
		all.WriteString(fmt.Sprintf("    %v:  %v\n", k, types[status.HistoryKind(k)]))
	}
	return all.String()
}

func (c *statusHistoryCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.outputContent, "type", "unit", fmt.Sprintf("Type of statuses to be displayed [%v]", supportedHistoryKindTypes()))
	f.IntVar(&c.backlogSize, "n", 0, "Returns the last N logs (cannot be combined with --days or --date)")
	f.IntVar(&c.backlogSizeDays, "days", 0, "Returns the logs for the past <days> days (cannot be combined with -n or --date)")
	f.StringVar(&c.backlogDate, "from-date", "", "Returns logs for any date after the passed one, the expected date format is YYYY-MM-DD (cannot be combined with -n or --days)")
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
		if c.outputContent != status.KindModel.String() {
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
				return errors.Annotatef(err, "invalid %s env var, expected true|false", osenv.JujuStatusIsoTimeEnvKey)
			}
		}
	}
	emptyDate := c.backlogDate == ""
	emptySize := c.backlogSize == 0
	emptyDays := c.backlogSizeDays == 0
	if emptyDate && emptySize && emptyDays {
		c.backlogSize = 20
	}
	if (!emptyDays && !emptySize) || (!emptyDays && !emptyDate) || (!emptySize && !emptyDate) {
		return errors.Errorf("backlog size, backlog date and backlog days back cannot be specified together")
	}
	if c.backlogDate != "" {
		var err error
		c.date, err = time.Parse("2006-01-02", c.backlogDate)
		if err != nil {
			return errors.Annotate(err, "parsing backlog date")
		}
	}

	kind := status.HistoryKind(c.outputContent)
	if kind.Valid() {
		return nil
	}
	return errors.Errorf("unexpected status type %q", c.outputContent)
}

// DetailedStatus holds status info about a machine or unit agent.
type DetailedStatus struct {
	Status  status.Status      `yaml:"status,omitempty" json:"status,omitempty"`
	Message string             `yaml:"message,omitempty" json:"message,omitempty"`
	Data    map[string]any     `yaml:"data,omitempty" json:"data,omitempty"`
	Since   *time.Time         `yaml:"since,omitempty" json:"since,omitempty"`
	Kind    status.HistoryKind `yaml:"type,omitempty" json:"type,omitempty"`
}

// History holds the status results.
type History []DetailedStatus

func (c *statusHistoryCommand) Run(ctx *cmd.Context) error {
	kind := status.HistoryKind(c.outputContent)
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
	var tag names.Tag
	switch kind {
	case status.KindModel:
		_, details, err := c.ModelDetails(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		tag = names.NewModelTag(details.ModelUUID)
	case status.KindUnit, status.KindWorkload, status.KindUnitAgent:
		if !names.IsValidUnit(c.entityName) {
			return errors.Errorf("%q is not a valid name for a %s", c.entityName, kind)
		}
		tag = names.NewUnitTag(c.entityName)
	case status.KindApplication, status.KindSAAS:
		if !names.IsValidApplication(c.entityName) {
			return errors.Errorf("%q is not a valid name for an application", c.entityName)
		}
		tag = names.NewApplicationTag(c.entityName)
	default:
		if !names.IsValidMachine(c.entityName) {
			return errors.Errorf("%q is not a valid name for a %s", c.entityName, kind)
		}
		tag = names.NewMachineTag(c.entityName)
	}

	clients, compat, err := c.getStatusHistoryClients(ctx, ctx)
	if err != nil {
		return err
	} else if len(clients) == 0 {
		return errors.New("no controller status-history clients available; is bootstrap still in progress?")
	}
	defer func() {
		for _, client := range clients {
			_ = client.Close()
		}
	}()

	var statuses []status.DetailedStatus
	for _, client := range clients {
		s, err := client.StatusHistory(ctx, kind, tag, filterArgs)
		// For compatibility with older controllers, if there is an error, but
		// there are statuses returned, we ignore the error, otherwise we
		// collect it.
		if err != nil {
			if len(s) == 0 && compat {
				return errors.Trace(err)
			}

			// Display any error, but continue to print status if some was
			// returned.
			fmt.Fprintf(ctx.Stderr, "%v\n", err)
		}

		statuses = append(statuses, s...)
	}

	// If there are no statuses at all, return an error.
	if len(statuses) == 0 {
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
	sort.Slice(history, func(i, j int) bool {
		a, b := history[i], history[j]
		if a.Since == nil && b.Since == nil {
			return a.Status.String() < b.Status.String()
		} else if a.Since == nil {
			return false
		} else if b.Since == nil {
			return true
		}
		return a.Since.Before(*b.Since)
	})
	return c.out.Write(ctx, history)
}

func (c *statusHistoryCommand) formatTabular(writer io.Writer, value any) error {
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

type warningLogger interface {
	Warningf(format string, args ...any)
}

func (c *statusHistoryCommand) getStatusHistoryClients(ctx context.Context, warningLogger warningLogger) ([]HistoryAPI, bool, error) {
	if c.clients != nil {
		return c.clients, true, nil
	}

	controllerClient, err := getControllerDetailsClient(ctx, c)
	if err != nil {
		return nil, false, errors.Annotatef(err, "getting controller addresses")
	}
	defer controllerClient.Close()

	// If we're connected to a HA facade that is less that 3, that indicates
	// that the controller details API is not supported, so we fall back to
	// using the address of the connected controller only.
	if controllerClient.BestAPIVersion() < 3 {
		clients, err := c.getStatusHistoryClient(ctx)
		return clients, true, err
	}

	// We're connected to a HA controller that supports the controller details
	// API, so we can get the addresses of all controllers.
	controllers, err := controllerClient.ControllerDetails(ctx)
	if errors.Is(err, errors.NotSupported) {
		clients, err := c.getStatusHistoryClient(ctx)
		return clients, true, err
	} else if err != nil {
		return nil, false, errors.Annotatef(err, "getting controller details")
	}

	var clients []HistoryAPI
	for _, details := range controllers {
		client, err := getStatusHistoryClientForAddresses(ctx, c, details.APIEndpoints)
		if len(controllers) > 1 && errors.Is(err, api.ConnectionFailure) {
			warningLogger.Warningf("cannot connect to status history client for controller %q at addresses %v: %v", details.ControllerID, details.APIEndpoints, err)
			continue
		} else if err != nil {
			return nil, false, errors.Annotatef(err, "getting status history client for controller %q", details.ControllerID)
		}
		clients = append(clients, client)
	}
	return clients, false, nil
}

func (c *statusHistoryCommand) getStatusHistoryClient(ctx context.Context) ([]HistoryAPI, error) {
	client, err := getStatusHistoryClient(ctx, c)
	if err != nil {
		return nil, err
	}
	return []HistoryAPI{client}, nil
}

// ControllerDetailsAPI provides access to the high availability facade.
type ControllerDetailsAPI interface {
	// ControllerDetails returns details about all controllers known to the
	// client.
	ControllerDetails(ctx context.Context) (map[string]highavailability.ControllerDetails, error)

	// BestAPIVersion returns the best API version supported by the server.
	BestAPIVersion() int

	// Close closes the API client.
	Close() error
}

var getControllerDetailsClient = func(ctx context.Context, c *statusHistoryCommand) (ControllerDetailsAPI, error) {
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return highavailability.NewClient(root), nil
}

var getStatusHistoryClient = func(ctx context.Context, c *statusHistoryCommand) (HistoryAPI, error) {
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client.NewClient(root, logger), nil
}

var getStatusHistoryClientForAddresses = func(ctx context.Context, c *statusHistoryCommand, addresses []string) (HistoryAPI, error) {
	root, err := c.NewAPIRootWithDialOpts(ctx, &api.DialOpts{
		DialTimeout: 5 * time.Second,
		Timeout:     30 * time.Second,
	}, addresses...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client.NewClient(root, logger), nil
}
