// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/trace"
	domainagenterrors "github.com/juju/juju/domain/agentbinary/errors"
	"github.com/juju/juju/domain/modelagent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
)

type SimpleStreamAgentBinaryStore struct {
	providerForAgentBinaryFinder providertracker.ProviderGetter[ProviderForAgentBinaryFinder]
	getPreferredSimpleStreams    PreferredSimpleStreamsFunc
	agentBinaryFilter            AgentBinaryFilter
	httpClient                   corehttp.HTTPClient
}

// NewSimpleStreamAgentBinaryStore returns a new instance of SimpleStreamAgentBinaryStore.
func NewSimpleStreamAgentBinaryStore(
	providerForAgentBinaryFinder providertracker.ProviderGetter[ProviderForAgentBinaryFinder],
	getPreferredSimpleStreams PreferredSimpleStreamsFunc,
	agentBinaryFilter AgentBinaryFilter,
	httpClient corehttp.HTTPClient,
) *SimpleStreamAgentBinaryStore {
	return &SimpleStreamAgentBinaryStore{
		providerForAgentBinaryFinder: providerForAgentBinaryFinder,
		getPreferredSimpleStreams:    getPreferredSimpleStreams,
		agentBinaryFilter:            agentBinaryFilter,
		httpClient:                   httpClient,
	}
}

// ProviderForAgentBinaryFinder is a subset of cloud provider.
type ProviderForAgentBinaryFinder interface {
	environs.BootstrapEnviron
}

// PreferredSimpleStreamsFunc is a function that returns the preferred streams
// for the given version and stream.
type PreferredSimpleStreamsFunc func(
	vers *semversion.Number,
	forceDevel bool,
	stream string,
) []string

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

// GetEnvironAgentBinariesFinder returns a function that can be used to find
// agent binaries from the simplestreams data sources.
func (s *SimpleStreamAgentBinaryStore) GetEnvironAgentBinariesFinder() EnvironAgentBinariesFinderFunc {
	return func(
		ctx context.Context,
		major,
		minor int,
		version semversion.Number,
		requestedStream string,
		filter coretools.Filter,
	) (_ coretools.List, err error) {
		ctx, span := trace.Start(ctx, trace.NameFromFunc())
		defer span.End()

		provider, err := s.providerForAgentBinaryFinder(ctx)
		if errors.Is(err, coreerrors.NotSupported) {
			return nil, errors.Errorf("getting provider for agent binary finder %w", err)
		}
		if err != nil {
			return nil, errors.Capture(err)
		}
		cfg := provider.Config()
		if requestedStream == "" {
			requestedStream = cfg.AgentStream()
		}

		streams := s.getPreferredSimpleStreams(&version, cfg.Development(), requestedStream)
		ssFetcher := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
		return s.agentBinaryFilter(ctx, ssFetcher, provider, major, minor, streams, filter)
	}
}

// GetAgentBinary retrieves the agent binary corresponding to the given version
// and stream from simple stream.
// The caller is responsible for closing the returned reader.
//
// The following errors may be returned:
// - [domainagenterrors.NotFound] if the agent binary metadata does not exist.
func (s *SimpleStreamAgentBinaryStore) GetAgentBinary(ctx context.Context, ver coreagentbinary.Version, stream modelagent.AgentStream) (io.ReadCloser, int64, string, error) {
	finder := s.GetEnvironAgentBinariesFinder()
	toolList, err := finder(ctx, ver.Number.Major, ver.Number.Minor, ver.Number, string(stream), coretools.Filter{
		Number: ver.Number,
		Arch:   ver.Arch,
	})
	if err != nil {
		return nil, 0, "", errors.Errorf("getting tool list for version %q", ver.String())
	}

	if len(toolList) == 0 {
		return nil, 0, "", errors.Errorf("getting agent binary for version %q", ver.String()).Add(domainagenterrors.NotFound)
	} else if len(toolList) != 1 {
		return nil, 0, "", errors.Errorf("multiple tools found for version: %q", ver.String())
	}

	tool := toolList[0]
	toolURL := tool.URL

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, toolURL, nil)
	if err != nil {
		return nil, 0, "", errors.Capture(err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, "", errors.Errorf("error fetching tool metadata for version %q: %w", ver.String(), err)
	} else if resp == nil {
		return nil, 0, "", errors.Errorf("error fetching tool metadata for version %q", ver.String())
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, "", errors.Errorf("bad HTTP response with status: %v", resp.Status)
	}

	// data read offset is 0.
	data, respSha256, size, err := s.tmpCacheAndHashWithKnownSize(resp.Body, tool.Size)
	if err != nil {
		return nil, 0, "", err
	}

	if size != tool.Size {
		return nil, 0, "", errors.Errorf("size mismatch for %s", tool.URL)
	}
	if respSha256 != tool.SHA256 {
		return nil, 0, "", errors.Errorf("hash mismatch for %s", tool.URL)
	}

	return data, size, respSha256, nil
}

func (s *SimpleStreamAgentBinaryStore) tmpCacheAndHashWithKnownSize(r io.Reader, size int64) (io.ReadCloser, string, int64, error) {
	tmpFile, err := os.CreateTemp("", "jujutools*")
	if err != nil {
		return nil, "", 0, err
	}
	tmpFilename := tmpFile.Name()
	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFilename)
	}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()

	lr := &io.LimitedReader{R: r, N: size}
	hasher := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmpFile, hasher), lr)
	if err != nil {
		return nil, "", 0, err
	}
	if n != size {
		return nil, "", 0, errors.Errorf("expected %d bytes but got %d bytes", size, n)
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return nil, "", 0, err
	}

	return &cleanupCloser{tmpFile, cleanup}, hex.EncodeToString(hasher.Sum(nil)), n, nil
}
