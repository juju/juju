// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/retry"
)

const (
	// defaultSnapStoreBaseURL is the snap store the client talks to unless
	// --controller-snap-store-url overrides it. The store base URL covers
	// channel/revision resolution; the snap itself is downloaded by the
	// machine's snapd during provisioning.
	defaultSnapStoreBaseURL = "https://api.snapcraft.io"

	// snapDeviceSeries is the device series header the store demands on its
	// v2 API, matching what snapd sends.
	snapDeviceSeries = "16"

	// snapStoreRequestTimeout is the per-request timeout for a single snap
	// store request. The retry loop below owns the overall budget, so this
	// only needs to be long enough for one request to a slow but healthy
	// store.
	snapStoreRequestTimeout = 30 * time.Second

	// snapStoreRetryAttempts is the maximum number of attempts (including the
	// initial) made before giving up on a store request.
	snapStoreRetryAttempts = 20

	// snapStoreRetryDelay is the initial delay before the first retry.
	snapStoreRetryDelay = time.Second

	// snapStoreRetryMaxDelay caps the exponential backoff between attempts.
	snapStoreRetryMaxDelay = 5 * time.Second

	// snapStoreRetryMaxDuration is the total time budget across all attempts
	// of a single store request.
	snapStoreRetryMaxDuration = 60 * time.Second
)

// snapStoreRevision is the store release the client resolved for a channel or
// pinned revision of the controller snap for one architecture. The client only
// reads metadata (version and revision); it never downloads the snap itself.
type snapStoreRevision struct {
	// Revision is the store-assigned revision of the snap.
	Revision int
	// Version is the snap's version: string as reported by the store.
	Version string
}

// snapStoreClient resolves the controller snap's version and revision from the
// snap store over HTTPS. The client resolves a channel or pinned revision for
// the bootstrap architecture and returns the resolved version and revision; the
// machine downloads that exact revision itself during provisioning.
type snapStoreClient struct {
	baseURL    string
	httpClient *http.Client
	clock      clock.Clock
}

func newSnapStoreClient(storeURL string) *snapStoreClient {
	baseURL := strings.TrimRight(storeURL, "/")
	if baseURL == "" {
		baseURL = defaultSnapStoreBaseURL
	}
	return &snapStoreClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: snapStoreRequestTimeout,
		},
		clock: clock.WallClock,
	}
}

// snapInfoResponse mirrors the subset of the store /v2/snaps/info response
// that this client consumes.
type snapInfoResponse struct {
	ChannelMap []snapInfoChannelEntry `json:"channel-map"`
}

type snapInfoChannelEntry struct {
	Channel  snapInfoChannel `json:"channel"`
	Revision int             `json:"revision"`
	Version  string          `json:"version"`
}

type snapInfoChannel struct {
	Track        string `json:"track"`
	Risk         string `json:"risk"`
	Architecture string `json:"architecture"`
}

// resolveChannel resolves a channel (e.g. "4.2/edge") to a version and
// revision for the given architecture. channel is already in track/risk form.
func (c *snapStoreClient) resolveChannel(ctx context.Context, snapName, arch, channel string) (snapStoreRevision, error) {
	split := strings.SplitN(channel, "/", 2)
	track, risk := split[0], ""
	if len(split) == 2 {
		risk = split[1]
	}

	entries, err := c.fetchInfo(ctx, snapName, arch)
	if err != nil {
		return snapStoreRevision{}, err
	}

	for _, e := range entries.ChannelMap {
		if e.Channel.Architecture != arch {
			continue
		}
		if e.Channel.Track != track {
			continue
		}
		if risk != "" && e.Channel.Risk != risk {
			continue
		}
		return snapStoreRevision{
			Revision: e.Revision,
			Version:  e.Version,
		}, nil
	}
	return snapStoreRevision{}, fmt.Errorf(
		"no controller snap %q revision available for channel %q on architecture %q",
		snapName, channel, arch,
	)
}

// resolveRevision resolves a pinned store revision to its version for the
// given architecture.
func (c *snapStoreClient) resolveRevision(ctx context.Context, snapName, arch string, revision int) (snapStoreRevision, error) {
	entries, err := c.fetchInfo(ctx, snapName, arch)
	if err != nil {
		return snapStoreRevision{}, err
	}

	for _, e := range entries.ChannelMap {
		if e.Channel.Architecture != arch {
			continue
		}
		if e.Revision != revision {
			continue
		}
		return snapStoreRevision{
			Revision: e.Revision,
			Version:  e.Version,
		}, nil
	}
	return snapStoreRevision{}, fmt.Errorf(
		"no controller snap %q revision %d available for architecture %q",
		snapName, revision, arch,
	)
}

// retryableStoreError wraps a store error that should be retried. The retry
// loop's IsFatalError recognises this type and continues retrying; any other
// error is treated as fatal.
type retryableStoreError struct{ err error }

func (e *retryableStoreError) Error() string { return e.err.Error() }
func (e *retryableStoreError) Unwrap() error { return e.err }

// fetchInfo returns the channel-map entries for the snap on the given
// architecture using the store /v2/snaps/info endpoint. It retries transient
// failures (429, 5xx, timeouts) with exponential backoff.
func (c *snapStoreClient) fetchInfo(ctx context.Context, snapName, arch string) (snapInfoResponse, error) {
	var lastInfo snapInfoResponse
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			info, retryable, err := c.doFetchInfo(ctx, snapName, arch)
			if err == nil {
				lastInfo = info
				return nil
			}
			if retryable {
				return &retryableStoreError{err: err}
			}
			return err
		},
		IsFatalError: func(err error) bool {
			var r *retryableStoreError
			return !errors.As(err, &r)
		},
		Attempts:    snapStoreRetryAttempts,
		Delay:       snapStoreRetryDelay,
		MaxDelay:    snapStoreRetryMaxDelay,
		MaxDuration: snapStoreRetryMaxDuration,
		BackoffFunc: retry.ExpBackoff(snapStoreRetryDelay, snapStoreRetryMaxDelay, 2.0, false),
		Clock:       c.clock,
		Stop:        ctx.Done(),
	})
	if err != nil {
		lastErr := retry.LastError(err)
		return snapInfoResponse{}, jujuerrors.Annotatef(lastErr, "querying snap store for %q", snapName)
	}
	return lastInfo, nil
}

// doFetchInfo performs a single request to the store /v2/snaps/info endpoint.
// It returns the response and a boolean indicating whether the error is
// retryable (429, 5xx, network timeout). The caller owns retry logic.
func (c *snapStoreClient) doFetchInfo(ctx context.Context, snapName, arch string) (snapInfoResponse, bool, error) {
	u := fmt.Sprintf("%s/v2/snaps/info/%s?architecture=%s&fields=version,revision,channel-map", c.baseURL, snapName, arch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return snapInfoResponse{}, false, jujuerrors.Annotatef(err, "creating store info request")
	}
	req.Header.Set("Snap-Device-Series", snapDeviceSeries)
	req.Header.Set("User-Agent", "juju")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		retryable := isRetryableNetworkError(err)
		return snapInfoResponse{}, retryable, fmt.Errorf("querying snap store for %q: %w", snapName, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if !isRetryableStatusCode(resp.StatusCode) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return snapInfoResponse{}, false, fmt.Errorf("snap store returned %s for %q: %s", resp.Status, snapName, strings.TrimSpace(string(body)))
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return snapInfoResponse{}, true, fmt.Errorf("snap store returned %s for %q: %s", resp.Status, snapName, strings.TrimSpace(string(body)))
	}

	var info snapInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return snapInfoResponse{}, false, jujuerrors.Annotatef(err, "decoding snap store response for %q", snapName)
	}
	return info, false, nil
}

// isRetryableStatusCode reports whether the status code indicates a transient
// failure that should be retried.
func isRetryableStatusCode(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// isRetryableNetworkError reports whether the error is a transient network
// failure (timeout or temporary DNS/connection error) that should be retried.
func isRetryableNetworkError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return true
	}
	return false
}

// resolveControllerSnap resolves the controller snap's version and revision for
// a store-based source mode. When revision is zero (no pinned revision) the
// channel is resolved for the bootstrap architecture; otherwise the pinned
// revision is targeted. The client reads only metadata from the store; the
// machine downloads the resolved revision itself during provisioning, so the
// exact resolved bytes are pinned and cannot drift between resolution and
// install.
//
// Declared as a var to allow test injection, mirroring BuildControllerSnap.
var resolveControllerSnap = func(
	ctx context.Context,
	storeURL, snapName, arch, channel string,
	revision int,
) (string, int, error) {
	client := newSnapStoreClient(storeURL)

	var target snapStoreRevision
	var err error
	if revision != 0 {
		target, err = client.resolveRevision(ctx, snapName, arch, revision)
	} else {
		target, err = client.resolveChannel(ctx, snapName, arch, channel)
	}
	if err != nil {
		return "", 0, jujuerrors.Annotate(err, "resolving controller snap in store")
	}
	return target.Version, target.Revision, nil
}
