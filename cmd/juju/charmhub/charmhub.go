// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
)

var logger = loggo.GetLogger("juju.cmd.juju.charmhub")

func newCharmHubCommand() *charmHubCommand {
	return &charmHubCommand{
		arches: arch.AllArches(),
		CharmHubClientFunc: func(config charmhub.Config) (CharmHubClient, error) {
			return charmhub.NewClient(config)
		},
	}
}

type charmHubCommand struct {
	cmd.CommandBase
	modelcmd.FilesystemCommand

	arch        string
	arches      arch.Arches
	base        string
	charmHubURL string

	// DEPRECATED: Use --base instead.
	series string

	CharmHubClientFunc func(charmhub.Config) (CharmHubClient, error)
}

func (c *charmHubCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.charmHubURL, "charmhub-url", charmhub.DefaultServerURL, "Specify the Charmhub URL for querying the store")
}

// Init initializes the info command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *charmHubCommand) Init(args []string) error {
	// If the architecture is empty, ensure we normalize it to all to prevent
	// complicated comparison checking.
	if c.arch == "" {
		c.arch = ArchAll
	}

	if c.arch != ArchAll && !c.arches.Contains(c.arch) {
		return errors.Errorf("unexpected architecture flag value %q, expected <%s>", c.arch, c.archArgumentList())
	}

	if urlFromEnv := os.Getenv("CHARMHUB_URL"); urlFromEnv != "" {
		c.charmHubURL = urlFromEnv
	}
	if c.charmHubURL == "" {
		c.charmHubURL = charmhub.DefaultServerURL
	}
	_, err := url.ParseRequestURI(c.charmHubURL)
	if err != nil {
		return errors.Annotatef(err, "invalid charmhub-url")
	}

	return nil
}

func (c *charmHubCommand) archArgumentList() string {
	archList := strings.Join(c.arches.StringList(), "|")
	return fmt.Sprintf("%s|%s", ArchAll, archList)
}
