// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/juju/errors"
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
}

func newSnapStoreClient(storeURL string) *snapStoreClient {
	baseURL := strings.TrimRight(storeURL, "/")
	if baseURL == "" {
		baseURL = defaultSnapStoreBaseURL
	}
	return &snapStoreClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
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

// fetchInfo returns the channel-map entries for the snap on the given
// architecture using the store /v2/snaps/info endpoint.
func (c *snapStoreClient) fetchInfo(ctx context.Context, snapName, arch string) (snapInfoResponse, error) {
	u := fmt.Sprintf("%s/v2/snaps/info/%s?architecture=%s&fields=version,revision,channel-map", c.baseURL, snapName, arch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return snapInfoResponse{}, errors.Annotatef(err, "creating store info request")
	}
	req.Header.Set("Snap-Device-Series", snapDeviceSeries)
	req.Header.Set("User-Agent", "juju")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return snapInfoResponse{}, errors.Annotatef(err, "querying snap store for %q", snapName)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return snapInfoResponse{}, fmt.Errorf("snap store returned %s for %q: %s", resp.Status, snapName, strings.TrimSpace(string(body)))
	}

	var info snapInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return snapInfoResponse{}, errors.Annotatef(err, "decoding snap store response for %q", snapName)
	}
	return info, nil
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
		return "", 0, errors.Annotate(err, "resolving controller snap in store")
	}
	return target.Version, target.Revision, nil
}
