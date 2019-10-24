// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"

	storageapi "github.com/juju/juju/api/storage"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
)

var logger = loggo.GetLogger("juju.cmd.juju.status")

type statusAPI interface {
	Status(patterns []string) (*params.FullStatus, error)
	Close() error
}

// NewStatusCommand returns a new command, which reports on the
// runtime state of various system entities.
func NewStatusCommand() cmd.Command {
	return modelcmd.Wrap(&statusCommand{
		checkProvidedIgnoredFlagF: func() set.Strings { return set.NewStrings() },
	})
}

// Clock defines the methods needed for the status command.
type Clock interface {
	After(time.Duration) <-chan time.Time
}

type statusCommand struct {
	modelcmd.ModelCommandBase
	out        cmd.Output
	patterns   []string
	isoTime    bool
	statusAPI  statusAPI
	storageAPI storage.StorageListAPI
	clock      Clock

	retryCount int
	retryDelay time.Duration

	color bool

	// relations indicates if 'relations' section is displayed
	relations bool

	// checkProvidedIgnoredFlagF indicates whether ignored options were provided by the user.
	checkProvidedIgnoredFlagF func() set.Strings

	// storage indicates if 'storage' section is displayed
	storage bool
}

var usageSummary = `
Reports the current status of the model, machines, applications and units.`[1:]

var usageDetails = `
By default (without argument), the status of the model, including all
applications and units will be output.

Application or unit names may be used as output filters (the '*' can be used as
a wildcard character). In addition to matched applications and units, related
machines, applications, and units will also be displayed. If a subordinate unit
is matched, then its principal unit will be displayed. If a principal unit is
matched, then all of its subordinates will be displayed.

Machine numbers may also be used as output filters. This will only display data
in each section relevant to the specified machines. For example, application
section will only contain the applications that have units on these machines, etc.

The available output formats are:

- tabular (default): Displays status in a tabular format with a separate table
	  for the model, machines, applications, relations (if any), storage (if any)
	  and units.
      Note: in this format, the AZ column refers to the cloud region's
      availability zone.
- {short|line|oneline}: List units and their subordinates. For each unit, the IP
      address and agent status are listed.
- summary: Displays the subnet(s) and port(s) the model utilises. Also displays
      aggregate information about:
      - Machines: total #, and # in each state.
      - Units: total #, and # in each state.
      - Applications: total #, and # exposed of each application.
- yaml: Displays information about the model, machines, applications, and units
      in structured YAML format.
- json: Displays information about the model, machines, applications, and units
      in structured JSON format.

In tabular format, 'Relations' section is not displayed by default.
Use --relations option to see this section. This option is ignored in all other
formats.

Examples:
    juju show-status
    juju show-status mysql
    juju show-status nova-*
    juju show-status --relations
    juju show-status --storage

See also:
    machines
    show-model
    show-status-log
    storage
`

func (c *statusCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "show-status",
		Args:    "[filter pattern ...]",
		Purpose: usageSummary,
		Doc:     usageDetails,
		Aliases: []string{"status"},
	})
}

func (c *statusCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.isoTime, "utc", false, "Display time as UTC in RFC3339 format")
	f.BoolVar(&c.color, "color", false, "Force use of ANSI color codes")

	f.BoolVar(&c.relations, "relations", false, "Show 'relations' section")
	f.BoolVar(&c.storage, "storage", false, "Show 'storage' section")

	f.IntVar(&c.retryCount, "retry-count", 3, "Number of times to retry API failures")
	f.DurationVar(&c.retryDelay, "retry-delay", 100*time.Millisecond, "Time to wait between retry attempts")

	c.checkProvidedIgnoredFlagF = func() set.Strings {
		ignoredFlagForNonTabularFormat := set.NewStrings(
			"relations",
			"storage",
		)
		provided := set.NewStrings()
		f.Visit(func(flag *gnuflag.Flag) {
			if ignoredFlagForNonTabularFormat.Contains(flag.Name) {
				provided.Add(flag.Name)
			}

		})
		return provided
	}

	defaultFormat := "tabular"

	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"short":   FormatOneline,
		"oneline": FormatOneline,
		"line":    FormatOneline,
		"tabular": c.FormatTabular,
		"summary": FormatSummary,
	})
}

func (c *statusCommand) Init(args []string) error {
	c.patterns = args
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
	if c.clock == nil {
		c.clock = clock.WallClock
	}
	return nil
}

var newAPIClientForStatus = func(c *statusCommand) (statusAPI, error) {
	if c.statusAPI == nil {
		api, err := c.NewAPIClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
		c.statusAPI = api
	}
	return c.statusAPI, nil
}

var newAPIClientForStorage = func(c *statusCommand) (storage.StorageListAPI, error) {
	if c.storageAPI == nil {
		root, err := c.NewAPIRoot()
		if err != nil {
			return nil, err
		}
		c.storageAPI = storageapi.NewClient(root)
	}
	return c.storageAPI, nil
}

func (c *statusCommand) close() {
	// We really don't care what the errors are if there are some.
	// The user can't do anything about it.  Just try.
	if c.statusAPI != nil {
		c.statusAPI.Close()
	}
	if c.storageAPI != nil {
		c.storageAPI.Close()
	}
	return
}

func (c *statusCommand) getStatus() (*params.FullStatus, error) {
	apiclient, err := newAPIClientForStatus(c)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiclient.Status(c.patterns)
}

func (c *statusCommand) getStorageInfo(ctx *cmd.Context) (*storage.CombinedStorage, error) {
	apiclient, err := newAPIClientForStorage(c)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return storage.GetCombinedStorageInfo(
		storage.GetCombinedStorageInfoParams{
			Context:         ctx,
			APIClient:       apiclient,
			Ids:             []string{},
			WantStorage:     true,
			WantVolumes:     true,
			WantFilesystems: true,
		})
}

func (c *statusCommand) Run(ctx *cmd.Context) error {
	defer c.close()

	// Always attempt to get the status at least once, and retry if it fails.
	status, err := c.getStatus()
	if err != nil && !modelcmd.IsModelMigratedError(err) {
		for i := 0; i < c.retryCount; i++ {
			// fun bit - make sure a new api connection is used for each new call
			c.SetModelAPI(nil)
			// Wait for a bit before retries.
			<-c.clock.After(c.retryDelay)
			status, err = c.getStatus()
			if err == nil || modelcmd.IsModelMigratedError(err) {
				break
			}
		}
	}

	if err != nil {
		if status == nil {
			// Status call completely failed, there is nothing to report
			return errors.Trace(err)
		}
		// Display any error, but continue to print status if some was returned
		fmt.Fprintf(ctx.Stderr, "%v\n", err)
	} else if status == nil {
		return errors.Errorf("unable to obtain the current status")
	}

	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	activeBranch, err := c.ActiveBranch()
	if err != nil {
		return errors.Trace(err)
	}

	showRelations := c.relations
	showStorage := c.storage
	if c.out.Name() != "tabular" {
		showRelations = true
		showStorage = true
		providedIgnoredFlags := c.checkProvidedIgnoredFlagF()
		if !providedIgnoredFlags.IsEmpty() {
			// For non-tabular formats this is redundant and needs to be mentioned to the user.
			joinedMsg := strings.Join(providedIgnoredFlags.SortedValues(), ", ")
			if providedIgnoredFlags.Size() > 1 {
				joinedMsg += " options are"
			} else {
				joinedMsg += " option is"
			}
			ctx.Infof("provided %s always enabled in non tabular formats", joinedMsg)
		}
	}
	formatterParams := newStatusFormatterParams{
		status:         status,
		controllerName: controllerName,
		outputName:     c.out.Name(),
		isoTime:        c.isoTime,
		showRelations:  showRelations,
		activeBranch:   activeBranch,
	}
	if showStorage {
		storageInfo, err := c.getStorageInfo(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		formatterParams.storage = storageInfo
		if storageInfo == nil || storageInfo.Empty() {
			if c.out.Name() == "tabular" {
				// hide storage section for tabular view if nothing to show.
				formatterParams.storage = nil
			}
		}
	}

	formatted, err := newStatusFormatter(formatterParams).format()
	if err != nil {
		return errors.Trace(err)
	}

	if err = c.out.Write(ctx, formatted); err != nil {
		return err
	}

	if !status.IsEmpty() {
		return nil
	}
	if len(c.patterns) == 0 {
		modelName, err := c.ModelIdentifier()
		if err != nil {
			return err
		}
		ctx.Infof("Model %q is empty.", modelName)
	} else {
		plural := func() string {
			if len(c.patterns) == 1 {
				return ""
			}
			return "s"
		}
		ctx.Infof("Nothing matched specified filter%v.", plural())
	}
	return nil
}

func (c *statusCommand) FormatTabular(writer io.Writer, value interface{}) error {
	return FormatTabular(writer, c.color, value)
}
