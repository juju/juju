// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"
	"net/http"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/domain/agentbinary"
	domainagenterrors "github.com/juju/juju/domain/agentbinary/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
)

var (
	headerAccept      = "Accept"
	headerContentType = "Content-Type"
)

// AgentBinaryFilter is a function that filters agent binaries based on the
// given parameters. It returns a list of agent binaries that match the filter
// criteria.
type AgentBinaryFilter func(
	ctx context.Context,
	ss envtools.SimplestreamsFetcher,
	env environs.BootstrapEnviron,
	majorVersion,
	minorVersion int,
	streams []string,
	filter coretools.Filter,
) (coretools.List, error)

// Doer represents a HTTP client capable of performing HTTP requests.
type Doer interface {
	// Do sends the HTTP request on behalf of the caller. See [http.Client.Do]
	// for a full description.
	Do(req *http.Request) (*http.Response, error)
}

// ProviderForAgentBinaryFinder is a subset of cloud provider.
type ProviderForAgentBinaryFinder interface {
	environs.BootstrapEnviron
}

// SimpleStreamsAgentBinaryStore fetches agent binaries from simplestreams
// using a provider-aware filter and an HTTP client.
type SimpleStreamsAgentBinaryStore struct {
	providerForAgentBinaryFinder providertracker.ProviderGetter[ProviderForAgentBinaryFinder]
	agentBinaryFilter            AgentBinaryFilter
	httpClient                   Doer
}

// NewSimpleStreamAgentBinaryStore returns a new instance of SimpleStreamsAgentBinaryStore.
func NewSimpleStreamAgentBinaryStore(
	providerForAgentBinaryFinder providertracker.ProviderGetter[ProviderForAgentBinaryFinder],
	agentBinaryFilter AgentBinaryFilter,
	httpClient Doer,
) *SimpleStreamsAgentBinaryStore {
	return &SimpleStreamsAgentBinaryStore{
		providerForAgentBinaryFinder: providerForAgentBinaryFinder,
		agentBinaryFilter:            agentBinaryFilter,
		httpClient:                   httpClient,
	}
}

// getPreferredFallbackStreams returns the preferred stream and fall back stream
// to use with the simplestream client.
func getPreferredFallbackStreams(stream agentbinary.Stream) []string {
	switch stream {
	case agentbinary.AgentStreamReleased:
		return []string{"released"}
	case agentbinary.AgentStreamProposed:
		return []string{"proposed", "released"}
	case agentbinary.AgentStreamDevel:
		return []string{"devel", "proposed", "released"}
	case agentbinary.AgentStreamTesting:
		return []string{"testing", "devel", "proposed", "released"}
	}
	return []string{}
}

const (
	gzipXContentType = "application/x-gzip"
	gzipContentType  = "application/gzip"
)

// GetAgentBinaryWithSHA256 retrieves the agent binary corresponding to the given version
// and stream from simple stream.
// The caller is responsible for closing the returned reader.
//
// The following errors may be returned:
// - [domainagenterrors.NotFound] if the agent binary metadata does not exist.
func (s *SimpleStreamsAgentBinaryStore) GetAgentBinaryWithSHA256(
	ctx context.Context,
	ver coreagentbinary.Version,
	stream agentbinary.Stream,
) (io.ReadCloser, int64, string, error) {
	foundToolsList, err := s.searchSimpleStreams(ctx, stream, ver)
	if err != nil {
		return nil, 0, "", errors.Errorf(
			"searching simple streams for %q in stream %q: %w",
			ver.String(), stream, err,
		)
	}

	if len(foundToolsList) == 0 {
		return nil, 0, "", errors.Errorf("getting agent binary for version %q", ver.String()).Add(domainagenterrors.NotFound)
	} else if len(foundToolsList) != 1 {
		return nil, 0, "", errors.Errorf(
			"multiple tools found for version: %q in stream %q, expected 1 result got %d",
			ver.String(), stream, len(foundToolsList),
		)
	}

	tool := foundToolsList[0]
	toolURL := tool.URL

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, toolURL, nil)
	if err != nil {
		return nil, 0, "", errors.Capture(err)
	}

	// We only accept gzip content types back.

	req.Header.Set(headerAccept, gzipXContentType+","+gzipContentType)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, "", errors.Errorf(
			"error fetching simple stream tools %q: %w", toolURL, err)
	}

	// Close on error paths only; if success, caller will own resp.Body.
	closeOnErr := func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}

	if resp.StatusCode == http.StatusNotFound {
		closeOnErr()
		return nil, 0, "", errors.Errorf("tool not found at simplestreams url %s", toolURL).
			Add(domainagenterrors.NotFound)
	}
	if resp.StatusCode == http.StatusNotAcceptable {
		closeOnErr()
		return nil, 0, "", errors.Errorf(
			"simplestreams url %q does not support expected content type %q",
			toolURL, gzipXContentType,
		)
	}
	if resp.StatusCode != http.StatusOK {
		closeOnErr()
		return nil, 0, "", errors.Errorf(
			"bad HTTP response from simplestreams url %q: %s",
			toolURL, resp.Status,
		)
	}

	if resp.Header.Get(headerContentType) != gzipXContentType &&
		resp.Header.Get(headerContentType) != gzipContentType {
		return nil, 0, "", errors.Errorf(
			"simplestreams url %q returned unexpected content type %q",
			toolURL, resp.Header.Get(headerContentType),
		)
	}

	return resp.Body, tool.Size, tool.SHA256, nil
}

// searchSimpleStreams prepares and conducts a simplestreams search for the
// required agent binary version in the given stream.
func (s *SimpleStreamsAgentBinaryStore) searchSimpleStreams(
	ctx context.Context,
	stream agentbinary.Stream,
	version coreagentbinary.Version,
) (coretools.List, error) {
	provider, err := s.providerForAgentBinaryFinder(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return nil, errors.Errorf("getting provider for agent binary finder %w", err)
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	major := version.Number.Major
	minor := version.Number.Minor
	filter := coretools.Filter{
		Arch:   version.Arch,
		Number: version.Number,
	}

	streams := getPreferredFallbackStreams(stream)
	ssFetcher := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	return s.agentBinaryFilter(ctx, ssFetcher, provider, major, minor, streams, filter)
}
