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
	coreseries "github.com/juju/juju/core/series"
)

var logger = loggo.GetLogger("juju.cmd.juju.charmhub")

func newCharmHubCommand() *charmHubCommand {
	return &charmHubCommand{
		CharmHubClientFunc: func(config charmhub.Config) (CharmHubClient, error) {
			return charmhub.NewClient(config)
		},
	}
}

type charmHubCommand struct {
	cmd.CommandBase
	modelcmd.FilesystemCommand

	arch        string
	base        string
	series      string
	charmHubURL string

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

	if c.arch != ArchAll && !arch.AllArches().Contains(c.arch) {
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
	archList := strings.Join(arch.AllArches().StringList(), "|")
	return fmt.Sprintf("%s|%s", ArchAll, archList)
}

// convertSeriesArgToBase converts the deprecated --series argument to a base.
func (c *charmHubCommand) convertSeriesArgToBase() error {
	if c.base != "all" && c.series != "" {
		return errors.Errorf("only one of --base or --series may be specified")
	}
	if c.series != "" {
		base, err := coreseries.GetBaseFromSeries(c.series)
		if err != nil {
			return errors.NotValidf("series %q", c.series)
		}
		c.base = base.String()
	}
	return nil
}
