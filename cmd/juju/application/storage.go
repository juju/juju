// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"io"
	"sort"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/config"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/storage"
)

const (
	appStorageConfigSummary = "Displays or sets storage directives for an application."

	appStorageConfigDoc = `
To view all storage directives for the given application:

    juju application-storage <application>

	By default, the config will be printed in a tabular format. You can instead
print it in ` + "`json`" + ` or ` + "`yaml`" + ` format using the ` + "`--format`" + ` flag:

   	juju application-storage <application> --format json
    juju application-storage <application> --format yaml

To view the directive of a single storage name:

    juju application-storage <application> <storage-name>

To set storage directives on an application:

    juju application-storage <application> <storagename1>=<storage-directive> <storagename2>=<storage-directive> ...

` + "`<storage-directive>` " + `describes to the charm how to refer to the storage,
and where to provision it from. ` + "`<storage-directive>` " + `takes the following form:

    <storage-name>[=<storage-configuration>]

` + "`<storage-name>` " + `is defined in the charm's ` + "`metadata.yaml` " + `file.

` + "`<storage-configuration>` " + `is a description of how Juju should provision storage
instances for the unit. They are made up of up to three parts: ` + "`<pool>`" + `,
` + "`<count>`" + `, and ` + "`<size>`" + `. They can be provided in any order, but we recommend the
following:

    <pool>,<count>,<size>

Each parameter is optional, so long as at least one is present. So the following
storage constraints are also valid:

    <pool>,<size>
    <count>,<size>
    <size>

` + "`<pool>` " + `is the storage pool to provision storage instances from. Must
be a name from ` + "`juju storage-pools`" + `.  The default pool is available via
executing ` + "`juju model-config storage-default-block-source`" + ` or ` + "`storage-default-filesystem-source`" + `.

` + "`<count>` " + `is the number of storage instances to provision from ` + "`<storage-pool>` " + `of
` + "`<size>`" + `. Must be a positive integer. The default count is ` + "`1`" + `. May be restricted
by the charm, which can specify a maximum number of storage instances per unit.

` + "`<size>` " + `is the number of bytes to provision per storage instance. Must be a
positive number, followed by a size suffix.  Valid suffixes include M, G, T,
and P.  Defaults to "1024M", or the which can specify a minimum size required
by the charm.
`
	appStorageConfigExamples = `
Print the storage directives for all storage names of the postgresql application:

    juju application-storage postgresql

Print the storage directives for the storage name 'pgdata' of the postgresql application:

    juju application-storage postgresql pgdata

Set the size to 10GiB, pool name to "rootfs", and count to 1 for the mysql application's 'database' storage:

    juju application-storage mysql database=10G,rootfs,1
	OR
    juju application-storage mysql database=rootfs,1,10G
	OR
    juju application-storage mysql database=1,10G,rootfs

If no size is provided, Juju uses the minimum size required by the charm. If the charm does not specify a minimum, the default is 1 GiB. 
This value is then applied when updating the application’s storage.

    juju application-storage mysql database=,rootfs,1

If no pool is provided, Juju selects the default storage pool from the model.
This pool will be recorded as the updated value for the application’s storage.

	juju application-storage mysql database=10G,,1

If no count is provided, Juju uses the minimum count required by the charm. 
That count will be used when updating the application’s storage.

	juju application-storage mysql database=10G,rootfs,
`
)

// NewStorageCommand wraps configCommand with sane application settings.
func NewStorageCommand() cmd.Command {
	applicationStorageConfigBase := config.ConfigCommandBase{
		Resettable: false,
	}
	return modelcmd.Wrap(&storageConfigCommand{configBase: applicationStorageConfigBase})
}

type storageConfigCommand struct {
	modelcmd.ModelCommandBase
	configBase config.ConfigCommandBase
	out        cmd.Output

	api             StorageDirectivesAPI
	applicationName string
}

// Info returns the name and usage details for the application storage command,
// providing guidance to the user.
// It implements the [cmd.Command] interface.
func (c *storageConfigCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:     "application-storage",
		Args:     "<application-name> [<storage-name>[={<size>,<pool>,<count>}]] ...",
		Purpose:  appStorageConfigSummary,
		Examples: appStorageConfigExamples,
		Doc:      appStorageConfigDoc,
		SeeAlso: []string{
			"storage",
			"storage-pools",
			"add-storage",
			"add-unit",
		},
	}

	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *storageConfigCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.configBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"json":    cmd.FormatJson,
		"tabular": formatConfigTabular,
		"yaml":    cmd.FormatYaml,
	})
}

// Init parses the command arguments and validates the application name.
// It implements the [cmd.Command] interface.
func (c *storageConfigCommand) Init(args []string) error {
	nArgs := len(args)
	if nArgs == 0 {
		return errors.Errorf("no application specified")
	}

	if !names.IsValidApplication(args[0]) {
		return errors.Errorf("invalid application name %q", args[0])
	}
	c.applicationName = args[0]
	return c.configBase.Init(args[1:])
}

// getAPI returns the API. This allows passing in a test configCommandAPI
// implementation.
func (c *storageConfigCommand) getAPI() (StorageDirectivesAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	client := application.NewClient(api)
	return client, nil
}

// Run executes the main logic for the application storage command.
// It processes the configured actions by either retrieving the
// storage directive for a single storage name, setting storage
// directives, or listing all storage directives for the given application.
// It implements the [cmd.Command] interface.
func (c *storageConfigCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	for _, action := range c.configBase.Actions {
		var err error
		switch action {
		case config.GetOne:
			err = c.getConfig(ctx, client)
		case config.SetArgs:
			err = c.setConfig(client, c.configBase.ValsToSet)
		default:
			err = c.getAllConfig(ctx, client)
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// StorageDirectivesAPI defines the API methods that the application storage command uses.
type StorageDirectivesAPI interface {
	Close() error
	GetApplicationStorageDirectives(applicationName string) (application.ApplicationStorageDirectives, error)
	UpdateApplicationStorageDirectives(applicationStorageUpdateParams application.ApplicationStorageUpdate) error
}

// setConfig sets the provided key/value pairs on the application.
func (c *storageConfigCommand) setConfig(client StorageDirectivesAPI, attrs config.Attrs) error {
	sd := make(map[string]storage.Constraints, len(attrs))
	for k, v := range attrs {
		// This should give us a string of the form "10G,rootfs,1".
		constraintsStr := fmt.Sprint(v)
		parsedCons, err := storage.ParseConstraints(constraintsStr)
		if err != nil {
			return errors.Annotatef(err, "parsing storage constraints for %q", k)
		}
		sd[k] = parsedCons
	}

	updateParams := application.ApplicationStorageUpdate{
		ApplicationTag:    names.NewApplicationTag(c.applicationName),
		StorageDirectives: sd,
	}

	return client.UpdateApplicationStorageDirectives(updateParams)
}

// getConfig writes the value of a single application config key to the cmd.Context.
func (c *storageConfigCommand) getConfig(ctx *cmd.Context, client StorageDirectivesAPI) error {
	applicationStorageInfo, err := client.GetApplicationStorageDirectives(c.applicationName)
	if err != nil {
		return err
	}

	if len(c.configBase.KeysToGet) == 0 {
		return errors.New("no storage key specified")
	}

	storeKey := c.configBase.KeysToGet[0]
	storageConsForKey, ok := applicationStorageInfo.StorageDirectives[storeKey]
	if !ok {
		return errors.NotFoundf("storage %q", storeKey)
	}

	// Convert it to the desired map format so that the output package can format it.
	storageConsForKeyMap := map[string]storage.Constraints{
		storeKey: storageConsForKey,
	}

	return c.out.Write(ctx, storageConsForKeyMap)
}

// getAllConfig returns the entire configuration for the selected application.
func (c *storageConfigCommand) getAllConfig(ctx *cmd.Context, client StorageDirectivesAPI) error {
	applicationStorageInfo, err := client.GetApplicationStorageDirectives(c.applicationName)
	if err != nil {
		return err
	}

	return c.out.Write(ctx, applicationStorageInfo.StorageDirectives)
}

// formatConfigTabular writes a tabular summary of config information.
func formatConfigTabular(writer io.Writer, value interface{}) error {
	configValues, ok := value.(map[string]storage.Constraints)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", map[string]storage.Constraints{}, value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{
		TabWriter: tw,
	}

	var valueNames []string
	for name := range configValues {
		valueNames = append(valueNames, name)
	}
	sort.Strings(valueNames)
	w.Println("Storage", "Pool", "Size", "Count")

	for _, name := range valueNames {
		info := configValues[name]
		w.Println(name, info.Pool, info.Size, info.Count)
	}

	tw.Flush()
	return nil
}
