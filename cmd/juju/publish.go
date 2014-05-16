// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/bzr"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
)

type PublishCommand struct {
	envcmd.EnvCommandBase
	URL       string
	CharmPath string

	// changePushLocation allows translating the branch location
	// for testing purposes.
	changePushLocation func(loc string) string

	pollDelay time.Duration
}

const publishDoc = `
<charm url> can be a charm URL, or an unambiguously condensed form of it;
the following forms are accepted:

For cs:precise/mysql
  cs:precise/mysql
  precise/mysql

For cs:~user/precise/mysql
  cs:~user/precise/mysql

There is no default series, so one must be provided explicitly when
informing a charm URL. If the URL isn't provided, an attempt will be
made to infer it from the current branch push URL.
`

func (c *PublishCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "publish",
		Args:    "[<charm url>]",
		Purpose: "publish charm to the store",
		Doc:     publishDoc,
	}
}

func (c *PublishCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.CharmPath, "from", ".", "path for charm to be published")
}

func (c *PublishCommand) Init(args []string) error {
	if len(args) == 0 {
		return nil
	}
	c.URL = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *PublishCommand) ChangePushLocation(change func(string) string) {
	c.changePushLocation = change
}

func (c *PublishCommand) SetPollDelay(delay time.Duration) {
	c.pollDelay = delay
}

// Wording guideline to avoid confusion: charms have *URLs*, branches have *locations*.

func (c *PublishCommand) Run(ctx *cmd.Context) (err error) {
	branch := bzr.New(ctx.AbsPath(c.CharmPath))
	if _, err := os.Stat(branch.Join(".bzr")); err != nil {
		return fmt.Errorf("not a charm branch: %s", branch.Location())
	}
	if err := branch.CheckClean(); err != nil {
		return err
	}

	var curl *charm.URL
	if c.URL == "" {
		if err == nil {
			loc, err := branch.PushLocation()
			if err != nil {
				return fmt.Errorf("no charm URL provided and cannot infer from current directory (no push location)")
			}
			curl, err = charm.Store.CharmURL(loc)
			if err != nil {
				return fmt.Errorf("cannot infer charm URL from branch location: %q", loc)
			}
		}
	} else {
		curl, err = charm.InferURL(c.URL, "")
		if err != nil {
			return err
		}
	}

	pushLocation := charm.Store.BranchLocation(curl)
	if c.changePushLocation != nil {
		pushLocation = c.changePushLocation(pushLocation)
	}

	repo, err := charm.InferRepository(curl.Reference, "/not/important")
	if err != nil {
		return err
	}
	if repo != charm.Store {
		return fmt.Errorf("charm URL must reference the juju charm store")
	}

	localDigest, err := branch.RevisionId()
	if err != nil {
		return fmt.Errorf("cannot obtain local digest: %v", err)
	}
	logger.Infof("local digest is %s", localDigest)

	ch, err := charm.ReadDir(branch.Location())
	if err != nil {
		return err
	}
	if ch.Meta().Name != curl.Name {
		return fmt.Errorf("charm name in metadata must match name in URL: %q != %q", ch.Meta().Name, curl.Name)
	}

	oldEvent, err := charm.Store.Event(curl, localDigest)
	if _, ok := err.(*charm.NotFoundError); ok {
		oldEvent, err = charm.Store.Event(curl, "")
		if _, ok := err.(*charm.NotFoundError); ok {
			logger.Infof("charm %s is not yet in the store", curl)
			err = nil
		}
	}
	if err != nil {
		return fmt.Errorf("cannot obtain event details from the store: %s", err)
	}

	if oldEvent != nil && oldEvent.Digest == localDigest {
		return handleEvent(ctx, curl, oldEvent)
	}

	logger.Infof("sending charm to the charm store...")

	err = branch.Push(&bzr.PushAttr{Location: pushLocation, Remember: true})
	if err != nil {
		return err
	}
	logger.Infof("charm sent; waiting for it to be published...")
	for {
		time.Sleep(c.pollDelay)
		newEvent, err := charm.Store.Event(curl, "")
		if _, ok := err.(*charm.NotFoundError); ok {
			continue
		}
		if err != nil {
			return fmt.Errorf("cannot obtain event details from the store: %s", err)
		}
		if oldEvent != nil && oldEvent.Digest == newEvent.Digest {
			continue
		}
		if newEvent.Digest != localDigest {
			// TODO Check if the published digest is in the local history.
			return fmt.Errorf("charm changed but not to local charm digest; publishing race?")
		}
		return handleEvent(ctx, curl, newEvent)
	}
}

func handleEvent(ctx *cmd.Context, curl *charm.URL, event *charm.EventResponse) error {
	switch event.Kind {
	case "published":
		curlRev := curl.WithRevision(event.Revision)
		logger.Infof("charm published at %s as %s", event.Time, curlRev)
		fmt.Fprintln(ctx.Stdout, curlRev)
	case "publish-error":
		return fmt.Errorf("charm could not be published: %s", strings.Join(event.Errors, "; "))
	default:
		return fmt.Errorf("unknown event kind %q for charm %s", event.Kind, curl)
	}
	return nil
}
