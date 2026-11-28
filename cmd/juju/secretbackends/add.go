// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"context"
	"fmt"
	"io"
	"maps"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/v4/keyvalues"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/client/secretbackends"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
)

type addSecretBackendCommand struct {
	modelcmd.ControllerCommandBase

	AddSecretBackendsAPIFunc func(ctx context.Context) (AddSecretBackendsAPI, error)

	Name        string
	BackendType string
	ImportID    string

	// Attributes from a file.
	ConfigFile cmd.FileVar
	// Attributes from key value args.
	KeyValueAttrs map[string]any
}

var addSecretBackendsDoc = `
Registers a store on the controller; it does not select a backend for a model.
Supply configuration as key=value arguments, a YAML mapping with ` + "`--config`" + `, or
both. Use ` + "`--config -`" + ` to read stdin. Arguments override keys from the file.
For a provider that supports token refresh, token-rotate takes a duration
of at least one hour. Omit it to register without requesting refresh.

The command prints no ID on success. Omit ` + "`--import-id`" + ` for a new registration;
supplying an existing ID can update that registration instead.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/secret-backend/
`

const addSecretBackendsExamples = `
Register a Vault backend named ` + "`myvault`" + ` using the settings in a private YAML file:

    juju add-secret-backend myvault vault --config vault_config.yaml

Register a Vault backend named ` + "`myvault`" + ` using the settings in a private YAML file and request a two-hour lifetime for Juju's Vault access token:

    juju add-secret-backend myvault vault --config vault_config.yaml token-rotate=2h
`

// AddSecretBackendsAPI is the secrets client API.
type AddSecretBackendsAPI interface {
	AddSecretBackend(ctx context.Context, backend secretbackends.CreateSecretBackend) error
	Close() error
}

// NewAddSecretBackendCommand returns a command to add a secret backend.
func NewAddSecretBackendCommand() cmd.Command {
	c := &addSecretBackendCommand{}
	c.AddSecretBackendsAPIFunc = c.secretBackendsAPI

	return modelcmd.WrapController(c)
}

func (c *addSecretBackendCommand) secretBackendsAPI(ctx context.Context) (AddSecretBackendsAPI, error) {
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return secretbackends.NewClient(root), nil

}

// Info implements cmd.Info.
func (c *addSecretBackendCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-secret-backend",
		Purpose:  "Add a new secret backend to the controller.",
		Doc:      addSecretBackendsDoc,
		Args:     "<backend-name> <backend-type> (--config <file> | <key>=<value> ...) [<key>=<value> ...]",
		Examples: addSecretBackendsExamples,
		SeeAlso: []string{
			"secret-backends",
			"remove-secret-backend",
			"show-secret-backend",
			"update-secret-backend",
		},
	})
}

// SetFlags implements cmd.SetFlags.
func (c *addSecretBackendCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(&c.ConfigFile, "config", "YAML configuration file, or - for stdin")
	f.StringVar(&c.ImportID, "import-id", "", "Add the backend with the specified ID")
}

func (c *addSecretBackendCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.New("must specify backend name and type")
	}
	c.Name = args[0]
	c.BackendType = args[1]
	// The remaining arguments are divided into keys to set.
	var err error
	if c.KeyValueAttrs, err = parseArgs(args[2:]); err != nil {
		return errors.Trace(err)
	}

	if len(c.KeyValueAttrs) == 0 && c.ConfigFile.Path == "" {
		return errors.New("must specify a config file or key values")
	}
	return nil
}

func parseArgs(args []string) (map[string]any, error) {
	keyValueAttrs := make(map[string]any)
	for _, arg := range args {
		splitArg := strings.SplitN(arg, "=", 2)
		if len(splitArg) != 2 {
			return nil, errors.NotValidf("key value %q", arg)
		}
		key := splitArg[0]
		if len(key) == 0 {
			return nil, errors.Errorf(`expected "key=value", got %q`, arg)
		}
		if _, exists := keyValueAttrs[key]; exists {
			return nil, keyvalues.DuplicateError(
				fmt.Sprintf("key %q specified more than once", key))
		}
		keyValueAttrs[key] = splitArg[1]
	}
	return keyValueAttrs, nil
}

func readFile(ctx *cmd.Context, configFile cmd.FileVar) (map[string]any, error) {
	attrs := make(map[string]any)
	if configFile.Path == "" {
		return attrs, nil
	}
	var (
		data []byte
		err  error
	)
	if configFile.Path == "-" {
		// Read from stdin
		data, err = io.ReadAll(ctx.Stdin)
	} else {
		// Read from file
		data, err = configFile.Read(ctx)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := yaml.Unmarshal(data, &attrs); err != nil {
		return nil, errors.Trace(err)
	}
	return attrs, nil
}

func parseTokenRotate(attrs map[string]any, zeroAllowed bool) (*time.Duration, error) {
	const tokenRotate = "token-rotate"
	tokenRotateStr, ok := attrs[tokenRotate]
	if ok {
		delete(attrs, tokenRotate)
		rotateInterval, err := time.ParseDuration(fmt.Sprintf("%s", tokenRotateStr))
		intervalSecs := rotateInterval / time.Second
		if err != nil {
			return nil, errors.Annotate(err, "invalid token rotate interval")
		}
		if intervalSecs == 0 {
			if !zeroAllowed {
				return nil, errors.NewNotValid(err, "token rotate interval cannot be 0")
			}
			return &rotateInterval, nil
		} else {
			if _, err := secrets.NextBackendRotateTime(time.Now(), rotateInterval); err != nil {
				return nil, errors.Trace(err)
			}
		}
		return &rotateInterval, nil
	}
	return nil, nil
}

// Run implements cmd.Run.
func (c *addSecretBackendCommand) Run(ctxt *cmd.Context) error {
	attrs, err := readFile(ctxt, c.ConfigFile)
	if err != nil {
		return errors.Trace(err)
	}
	maps.Copy(attrs, c.KeyValueAttrs)

	tokenRotateInterval, err := parseTokenRotate(attrs, false)
	if err != nil {
		return errors.Trace(err)
	}
	p, err := provider.Provider(c.BackendType)
	if err != nil {
		return errors.Annotatef(err, "invalid secret backend %q", c.BackendType)
	}
	configValidator, ok := p.(provider.ProviderConfig)
	if ok {
		err = configValidator.ValidateConfig(nil, attrs, tokenRotateInterval)
		if err != nil {
			return errors.Annotate(err, "invalid provider config")
		}
	}

	backend := secretbackends.CreateSecretBackend{
		ID:                  c.ImportID,
		Name:                c.Name,
		BackendType:         c.BackendType,
		TokenRotateInterval: tokenRotateInterval,
		Config:              attrs,
	}
	api, err := c.AddSecretBackendsAPIFunc(ctxt)
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	err = api.AddSecretBackend(ctxt, backend)
	return errors.Trace(err)
}
