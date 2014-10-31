package server

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"
)

const serverTrustCommandDoc = `
Trust a remote identity provider.

The Juju server will trust incoming API connections from a remote identity
provider, given its public key (a base64-encoded byte sequence) and its
location URL.

The identity provider is stored in the Juju server state.

Examples:
  # Set identity provider
  $ juju server trust eu91oohvHkItbg6wEoIBPcXifeCsbGQ8gu4kxp5YlCk= id-service:8443

  # Show identity provider trust relationship
  $ juju server trust
  eu91oohvHkItbg6wEoIBPcXifeCsbGQ8gu4kxp5YlCk= id-service:8443
`

// TrustCommand sets the the identity provider for a Juju server.
type TrustCommand struct {
	ServerCommandBase
	PublicKey string
	Location  string

	showTrust bool
}

// Info implements Command.Info.
func (c *TrustCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "trust",
		Args:    "<public key> <location>",
		Purpose: "trust a remote identity provider",
		Doc:     serverTrustCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *TrustCommand) SetFlags(f *gnuflag.FlagSet) {
}

// Init implements Command.Init.
func (c *TrustCommand) Init(args []string) error {
	if len(args) < 2 {
		c.showTrust = true
		return nil
	}
	c.PublicKey, c.Location = args[0], args[1]

	pkBytes, err := base64.URLEncoding.DecodeString(c.PublicKey)
	if err != nil {
		return err
	}
	if len(pkBytes) == 0 {
		return fmt.Errorf("invalid public key length")
	}

	_, err = url.Parse(c.Location)
	if err != nil {
		return err
	}
	return nil
}

func (c *TrustCommand) getServerAdminAPI() (ServerAdminAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewServerAdminClient()
}

var (
	getServerTrustAPI = (*TrustCommand).getServerAdminAPI
)

// Run implements Command.Run.
func (c *TrustCommand) Run(ctx *cmd.Context) error {
	client, err := getServerTrustAPI(c)
	if err != nil {
		return err
	}
	defer client.Close()

	if c.showTrust {
		info, err := client.IdentityProvider()
		if err != nil {
			return err
		}
		if info == nil {
			fmt.Fprintln(ctx.Stderr, "not set")
		} else {
			fmt.Fprintf(ctx.Stdout, "%s\t%s\n", info.PublicKey, info.Location)
		}
		return nil
	} else {
		return client.SetIdentityProvider(c.PublicKey, c.Location)
	}
}
