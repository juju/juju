// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/v3/keyvalues"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/client/secretbackends"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
)

type addSecretBackendCommand struct {
	modelcmd.ControllerCommandBase

	AddSecretBackendsAPIFunc func() (AddSecretBackendsAPI, error)

	Name        string
	BackendType string
	ImportID    string

	// Attributes from a file.
	ConfigFile cmd.FileVar
	// Attributes from key value args.
	KeyValueAttrs map[string]interface{}
}

var addSecretBackendsDoc = `
Adds a new secret backend for storing secret content.

You must specify a name for the backend and its type,
followed by any necessary backend specific config values.
Config may be specified as key values ot read from a file.
Any key values override file content if both are specified.

To rotate the backend access credential/token (if specified), use
the ` + "`token-rotate` " + `config and supply a duration.

`

const addSecretBackendsExamples = `
    juju add-secret-backend myvault vault --config /path/to/cfg.yaml
    juju add-secret-backend myvault vault token-rotate=10m --config /path/to/cfg.yaml
    juju add-secret-backend myvault vault endpoint=https://vault.io:8200 token=s.1wshwhw
`

// AddSecretBackendsAPI is the secrets client API.
type AddSecretBackendsAPI interface {
	AddSecretBackend(backend secretbackends.CreateSecretBackend) error
	Close() error
}

// NewAddSecretBackendCommand returns a command to add a secret backend.
func NewAddSecretBackendCommand() cmd.Command {
	c := &addSecretBackendCommand{}
	c.AddSecretBackendsAPIFunc = c.secretBackendsAPI

	return modelcmd.WrapController(c)
}

func (c *addSecretBackendCommand) secretBackendsAPI() (AddSecretBackendsAPI, error) {
	root, err := c.NewAPIRoot()
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
		Args:     "<backend-name> <backend-type>",
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
	f.Var(&c.ConfigFile, "config", "path to yaml-formatted configuration file")
	f.StringVar(&c.ImportID, "import-id", "", "add the backend with the specified id")
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

func parseArgs(args []string) (map[string]interface{}, error) {
	keyValueAttrs := make(map[string]interface{})
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

func readFile(ctx *cmd.Context, configFile cmd.FileVar) (map[string]interface{}, error) {
	attrs := make(map[string]interface{})
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

func parseTokenRotate(attrs map[string]interface{}, zeroAllowed bool) (*time.Duration, error) {
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
	for k, v := range c.KeyValueAttrs {
		attrs[k] = v
	}

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
	api, err := c.AddSecretBackendsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	err = api.AddSecretBackend(backend)
	return errors.Trace(err)
}
