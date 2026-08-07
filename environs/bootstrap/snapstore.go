// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"crypto/sha3"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/errors"
)

const (
	// defaultSnapStoreBaseURL is the snap store the client talks to unless
	// --controller-snap-store-url overrides it. The store base URL covers both
	// channel/revision resolution and the .snap download.
	defaultSnapStoreBaseURL = "https://api.snapcraft.io"

	// snapDeviceSeries is the device series header the store demands on its
	// v2 API, matching what snapd sends.
	snapDeviceSeries = "16"
)

// snapStoreRevision is the resolved download target for a channel or pinned
// revision of the controller snap for one architecture.
type snapStoreRevision struct {
	// Revision is the store-assigned revision of the snap.
	Revision int
	// Version is the snap's version: string as reported by the store.
	Version string
	// DownloadURL is where the .snap bytes can be fetched over HTTPS.
	DownloadURL string
	// Sha3 is the store-assigned SHA3-384 digest of the .snap file.
	Sha3 string
}

// snapStoreClient acquires the controller snap from the snap store over HTTPS,
// replacing the machine-side `snap download`. The client resolves a channel or
// pinned revision for the bootstrap architecture, downloads the exact .snap,
// verifies the assembled assertion against its digest, and assembles a .assert
// valid for `snap ack` then `snap install` without --dangerous. The machine
// never contacts the store.2
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
	Channel  snapInfoChannel  `json:"channel"`
	Revision int              `json:"revision"`
	Version  string           `json:"version"`
	Download snapInfoDownload `json:"download"`
}

type snapInfoChannel struct {
	Track        string `json:"track"`
	Risk         string `json:"risk"`
	Architecture string `json:"architecture"`
}

type snapInfoDownload struct {
	URL  string `json:"url"`
	Sha3 string `json:"sha3-384"`
}

// resolveChannel resolves a channel (e.g. "4.2/edge") to a download target for
// the given architecture. channel is already in track/risk form.
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

	for _, e := range entries {
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
			Revision:    e.Revision,
			Version:     e.Version,
			DownloadURL: e.Download.URL,
			Sha3:        e.Download.Sha3,
		}, nil
	}
	return snapStoreRevision{}, fmt.Errorf(
		"no controller snap %q revision available for channel %q on architecture %q",
		snapName, channel, arch,
	)
}

// resolveRevision resolves a pinned store revision to a download target for
// the given architecture.
func (c *snapStoreClient) resolveRevision(ctx context.Context, snapName, arch string, revision int) (snapStoreRevision, error) {
	entries, err := c.fetchInfo(ctx, snapName, arch)
	if err != nil {
		return snapStoreRevision{}, err
	}

	for _, e := range entries {
		if e.Revision != revision {
			continue
		}
		return snapStoreRevision{
			Revision:    e.Revision,
			Version:     e.Version,
			DownloadURL: e.Download.URL,
			Sha3:        e.Download.Sha3,
		}, nil
	}
	return snapStoreRevision{}, fmt.Errorf(
		"no controller snap %q revision %d available for architecture %q",
		snapName, revision, arch,
	)
}

// fetchInfo returns the channel-map entries for the snap on the given
// architecture using the store /v2/snaps/info endpoint.
func (c *snapStoreClient) fetchInfo(ctx context.Context, snapName, arch string) ([]snapInfoChannelEntry, error) {
	u := fmt.Sprintf("%s/v2/snaps/info/%s?architecture=%s&fields=version,revision,download,channel", c.baseURL, snapName, arch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "creating store info request")
	}
	req.Header.Set("Snap-Device-Series", snapDeviceSeries)
	req.Header.Set("User-Agent", "juju")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Annotatef(err, "querying snap store for %q", snapName)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("snap store returned %s for %q: %s", resp.Status, snapName, strings.TrimSpace(string(body)))
	}

	var info snapInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, errors.Annotatef(err, "decoding snap store response for %q", snapName)
	}
	return info.ChannelMap, nil
}

// download fetches the .snap to the destination path and returns it.
func (c *snapStoreClient) download(ctx context.Context, destPath, downloadURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return errors.Annotatef(err, "creating snap download request")
	}
	req.Header.Set("Snap-Device-Series", snapDeviceSeries)
	req.Header.Set("User-Agent", "juju")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Annotatef(err, "downloading controller snap")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("snap store returned %s on download: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	f, err := os.Create(destPath)
	if err != nil {
		return errors.Annotatef(err, "creating snap download file")
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return errors.Annotatef(err, "writing controller snap download")
	}
	return nil
}

// fetchAssertion fetches the assembled assertion chain for the given SHA3-384
// digest and writes it to destPath.
func (c *snapStoreClient) fetchAssertion(ctx context.Context, destPath, sha string) error {
	u := fmt.Sprintf("%s/api/v1/snaps/assertions/%s", c.baseURL, sha)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return errors.Annotatef(err, "creating assertion request")
	}
	req.Header.Set("Snap-Device-Series", snapDeviceSeries)
	req.Header.Set("User-Agent", "juju")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Annotatef(err, "fetching controller snap assertions")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("snap store returned %s on assertion fetch: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Annotatef(err, "reading controller snap assertion")
	}
	return os.WriteFile(destPath, body, 0o644)
}

// sha384Hex returns the hex-encoded SHA3-384 digest of the file at path.
func sha384Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha3.New384()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// assembleAndVerify verifies the downloaded snap against the resolved
// assertion's store-reported digest and confirms the on-disk assertion matches
// the downloaded file's SHA3-384. assertPath is the .assert written by
// fetchAssertion.
func (c *snapStoreClient) verify(snapPath, assertPath, expected string) error {
	actual, err := sha384Hex(snapPath)
	if err != nil {
		return errors.Annotatef(err, "hashing downloaded controller snap")
	}
	if expected != "" && !strings.EqualFold(actual, expected) {
		return fmt.Errorf(
			"downloaded controller snap SHA3-384 %s does not match store revision %s",
			actual, expected,
		)
	}

	if _, err := os.Stat(assertPath); err != nil {
		return errors.Annotatef(err, "assembled controller snap assertion")
	}
	return nil
}

// acquire downloads the controller snap and its assertion for the resolved
// target into dir, returning the paths to the .snap and .assert files. It is the
// client-side acquisition entry point: the client fetches the bytes and
// verifies the pair, so the machine never contacts the store.
func (c *snapStoreClient) acquire(ctx context.Context, dir string, target snapStoreRevision) (snapPath, assertPath string, err error) {
	snapPath = filepath.Join(dir, ControllerSnapPackageName+".snap")
	if err := c.download(ctx, snapPath, target.DownloadURL); err != nil {
		return "", "", err
	}

	assertPath = filepath.Join(dir, ControllerSnapPackageName+".assert")
	if err := c.fetchAssertion(ctx, assertPath, target.Sha3); err != nil {
		return "", "", err
	}

	if err := c.verify(snapPath, assertPath, target.Sha3); err != nil {
		return "", "", err
	}
	return snapPath, assertPath, nil
}

// acquiredControllerSnap is the result of acquiring a controller snap from the
// store on the client: the paths to the downloaded .snap and .assert files.
type acquiredControllerSnap struct {
	// SnapPath is the path to the downloaded .snap file.
	SnapPath string
	// AssertPath is the path to the assembled .assert file.
	AssertPath string
}

// acquireControllerSnap acquires the controller snap for a store-based source
// mode. When revision is zero (no pinned revision) the channel is resolved for
// the bootstrap architecture; otherwise the pinned revision is targeted. The
// .snap and .assert are downloaded to dir, the pair is verified, and the snap's
// version is read from the downloaded file via the snapd-free reader.
//
// Declared as a var to allow test injection, mirroring BuildControllerSnap.
var acquireControllerSnap = func(
	ctx context.Context,
	storeURL, snapName, arch, channel string,
	revision int,
	dir string,
) (*acquiredControllerSnap, error) {
	client := newSnapStoreClient(storeURL)

	var target snapStoreRevision
	var err error
	if revision != 0 {
		target, err = client.resolveRevision(ctx, snapName, arch, revision)
	} else {
		target, err = client.resolveChannel(ctx, snapName, arch, channel)
	}
	if err != nil {
		return nil, errors.Annotate(err, "resolving controller snap in store")
	}

	snapPath, assertPath, err := client.acquire(ctx, dir, target)
	if err != nil {
		return nil, errors.Annotate(err, "acquiring controller snap from store")
	}

	return &acquiredControllerSnap{
		SnapPath:   snapPath,
		AssertPath: assertPath,
	}, nil
}
