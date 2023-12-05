// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"fmt"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
)

// ResourceRetryClient is a wrapper around a Juju repository client that
// retries GetResource() calls.
type ResourceRetryClient struct {
	ResourceGetter
	retryArgs retry.CallArgs
}

func newRetryClient(client ResourceGetter) *ResourceRetryClient {
	retryArgs := retry.CallArgs{
		// (anastasiamac 2017-05-25) This might not work as the error types
		// may be lost after a call to some clients.
		IsFatalError: func(err error) bool {
			return errors.Is(err, errors.NotFound) || errors.Is(err, errors.NotValid)
		},
		// We don't want to retry for ever.
		// If we cannot get a resource after trying a few times,
		// most likely user intervention is needed.
		Attempts: 3,
		// A one minute gives enough time for potential connection
		// issues to sort themselves out without making the caller wait
		// for an exceptional amount of time.
		Delay: 1 * time.Minute,
		Clock: clock.WallClock,
	}
	return &ResourceRetryClient{
		ResourceGetter: client,
		retryArgs:      retryArgs,
	}
}

// GetResource returns a reader for the resource's data.
func (client ResourceRetryClient) GetResource(req ResourceRequest) (ResourceData, error) {
	args := client.retryArgs // a copy

	var data ResourceData
	args.Func = func() error {
		var err error
		data, err = client.ResourceGetter.GetResource(req)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	var channelStr string
	stChannel := req.CharmID.Origin.Channel
	if stChannel != nil {
		// Empty string is valid for CharmStore charms.
		channel, err := charm.MakeChannel(stChannel.Track, stChannel.Risk, stChannel.Branch)
		if err != nil {
			return data, errors.Trace(err)
		}
		channelStr = fmt.Sprintf("channel (%v), ", channel.String())
	}

	var lastErr error
	args.NotifyFunc = func(err error, i int) {
		// Remember the error we're hiding and then retry!
		logger.Warningf("attempt %d/%d to download resource %q from charm store [%scharm (%v), resource revision (%v)] failed with error (will retry): %v",
			i, client.retryArgs.Attempts, req.Name, channelStr, req.CharmID.URL, req.Revision, err)
		logger.Tracef("resource get error stack: %v", errors.ErrorStack(err))
		lastErr = err
	}

	err := retry.Call(args)
	if retry.IsAttemptsExceeded(err) {
		return data, errors.Annotate(lastErr, "failed after retrying")
	}
	if err != nil {
		return data, errors.Trace(err)
	}

	return data, nil
}
