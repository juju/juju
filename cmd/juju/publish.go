package main

import (
	"fmt"
	"launchpad.net/juju-core/bzr"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"
	"os"
	"strings"
	"time"
)

type PublishCommand struct {
	EnvCommandBase
	URL      string
	RepoPath string // defaults to JUJU_REPOSITORY

	// changePushLocation allows translating the branch location
	// for testing purposes.
	changePushLocation func(loc string) string

	pollDelay time.Duration
}

const publishDoc = `
<charm url> can be a charm URL, or an unambiguously condensed form of it;
assuming a current default series of "precise", the following forms will be
accepted.

For cs:precise/mysql
  mysql
  precise/mysql

For cs:~user/precise/mysql
  cs:~user/mysql

If <charm url> isn't provided, an attempt will be made to infer it from
the current branch push URL.
`

func (c *PublishCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "publish",
		Args:    "[<charm url>]",
		Purpose: "publish the charm in $PWD to the store",
		Doc:     publishDoc,
	}
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
	branch := bzr.New(".")
	if _, err := os.Stat(branch.Join(".bzr")); err != nil {
		return fmt.Errorf("publish must be run from within a charm branch")
	}
	if err := branch.MustBeClean(); err != nil {
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
		// TODO Long term we likely can't support a default series here.
		// InferURL must be able to take an empty default series and error
		// out if it can't work without one.
		curl, err = charm.InferURL(c.URL, "precise")
		if err != nil {
			return err
		}
	}

	pushLocation := charm.Store.BranchLocation(curl)
	if c.changePushLocation != nil {
		pushLocation = c.changePushLocation(pushLocation)
	}

	repo, err := charm.InferRepository(curl, ctx.AbsPath(c.RepoPath))
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
	log.Infof("local digest is %s", localDigest)

	oldEvent, err := charm.Store.Event(curl, localDigest)
	if _, ok := err.(*charm.NotFoundError); ok {
		oldEvent, err = charm.Store.Event(curl, "")
		if _, ok := err.(*charm.NotFoundError); ok {
			log.Infof("charm %s is not yet in the store", curl)
			err = nil
		}
	}
	if err != nil {
		return fmt.Errorf("cannot obtain event details from the store: %s", err)
	}

	if oldEvent != nil && oldEvent.Digest == localDigest {
		return handleEvent(ctx, curl, oldEvent)
	}

	log.Infof("sending charm to the charm store...")

	err = branch.Push(&bzr.PushAttr{Location: pushLocation, Remember: true})
	if err != nil {
		return err
	}
	log.Infof("charm sent; waiting for it to be published...")
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
	return nil
}

func handleEvent(ctx *cmd.Context, curl *charm.URL, event *charm.EventResponse) error {
	switch event.Kind {
	case "published":
		curlRev := curl.WithRevision(event.Revision)
		log.Infof("charm published at %s as %s", event.Time, curlRev)
		fmt.Fprintln(ctx.Stdout, curlRev)
	case "publish-error":
		return fmt.Errorf("charm could not be published: %s", strings.Join(event.Errors, "; "))
	default:
		return fmt.Errorf("unknown event kind %q for charm %s", event.Kind, curl)
	}
	return nil
}
