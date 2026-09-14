// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package identityfilewriter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	dt "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/identityfilewriter"
)

// ManifoldSuite covers the jujud-only controller manifold.
type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

type stubSystemIdentityReader struct {
	values identityfilewriter.SystemIdentityValues
	err    error
}

func (s stubSystemIdentityReader) SystemIdentityValues() (identityfilewriter.SystemIdentityValues, error) {
	return s.values, s.err
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &ManifoldSuite{})
}

func validManifoldConfig(identityPath string) identityfilewriter.ManifoldConfig {
	return identityfilewriter.ManifoldConfig{
		SystemIdentityReader: stubSystemIdentityReader{values: identityfilewriter.SystemIdentityValues{
			SystemIdentity:     "test-ssh-key",
			SystemIdentityPath: identityPath,
		}},
		NewWorker: identityfilewriter.NewWorker,
	}
}

// TestManifold_NoInputs asserts that the controller manifold lists no direct
// dependency inputs (no api-caller, no agent).
func (s *ManifoldSuite) TestManifold_NoInputs(c *tc.C) {
	dir := c.MkDir()
	cfg := validManifoldConfig(filepath.Join(dir, "system-identity"))
	manifold := identityfilewriter.Manifold(cfg)
	c.Check(manifold.Inputs, tc.HasLen, 0)
}

// TestManifold_Validate_EmptyPath ensures Validate rejects an empty
// SystemIdentityPath.
func (s *ManifoldSuite) TestManifold_Validate_EmptyPath(c *tc.C) {
	cfg := identityfilewriter.ManifoldConfig{
		SystemIdentityReader: stubSystemIdentityReader{values: identityfilewriter.SystemIdentityValues{
			SystemIdentity:     "some-key",
			SystemIdentityPath: "",
		}},
		NewWorker: identityfilewriter.NewWorker,
	}
	c.Check(cfg.Validate(), tc.ErrorIsNil)
	c.Check(cfg.SystemIdentityReader.(stubSystemIdentityReader).values.Validate(), tc.ErrorMatches, `empty SystemIdentityPath not valid`)
}

// TestManifold_Validate_NilNewWorker ensures Validate rejects a nil NewWorker.
func (s *ManifoldSuite) TestManifold_Validate_NilNewWorker(c *tc.C) {
	cfg := identityfilewriter.ManifoldConfig{
		SystemIdentityReader: stubSystemIdentityReader{values: identityfilewriter.SystemIdentityValues{
			SystemIdentity:     "some-key",
			SystemIdentityPath: "/tmp/system-identity",
		}},
		NewWorker: nil,
	}
	c.Check(cfg.Validate(), tc.ErrorMatches, `nil NewWorker not valid`)
}

// TestManifold_Start_WritesFile asserts that the worker writes the system
// identity file when SystemIdentity is non-empty.
func (s *ManifoldSuite) TestManifold_Start_WritesFile(c *tc.C) {
	dir := c.MkDir()
	identityPath := filepath.Join(dir, "system-identity")
	cfg := validManifoldConfig(identityPath)
	manifold := identityfilewriter.Manifold(cfg)

	w, err := manifold.Start(c.Context(), dt.StubGetter(nil))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.Not(tc.IsNil))
	w.Kill()
	c.Assert(w.Wait(), tc.ErrorIsNil)

	data, err := os.ReadFile(identityPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "test-ssh-key")

	info, err := os.Stat(identityPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Mode().Perm(), tc.Equals, os.FileMode(0600))
}

// TestManifold_Start_EmptyIdentityRemovesFile asserts that the worker removes
// the system identity file when SystemIdentity is empty.
func (s *ManifoldSuite) TestManifold_Start_EmptyIdentityRemovesFile(c *tc.C) {
	dir := c.MkDir()
	identityPath := filepath.Join(dir, "system-identity")

	// Create the file first.
	err := os.WriteFile(identityPath, []byte("old-key"), 0600)
	c.Assert(err, tc.ErrorIsNil)

	cfg := identityfilewriter.ManifoldConfig{
		SystemIdentityReader: stubSystemIdentityReader{values: identityfilewriter.SystemIdentityValues{
			SystemIdentity:     "",
			SystemIdentityPath: identityPath,
		}},
		NewWorker: identityfilewriter.NewWorker,
	}
	manifold := identityfilewriter.Manifold(cfg)

	w, err := manifold.Start(c.Context(), dt.StubGetter(nil))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.Not(tc.IsNil))
	w.Kill()
	c.Assert(w.Wait(), tc.ErrorIsNil)

	_, err = os.Stat(identityPath)
	c.Check(os.IsNotExist(err), tc.IsTrue)
}

// TestManifold_Start_EmptyIdentityNoFileIsIdempotent asserts that the worker
// succeeds when SystemIdentity is empty and the file does not exist.
func (s *ManifoldSuite) TestManifold_Start_EmptyIdentityNoFileIsIdempotent(c *tc.C) {
	dir := c.MkDir()
	identityPath := filepath.Join(dir, "system-identity")
	// File does not exist — no pre-creation.

	cfg := identityfilewriter.ManifoldConfig{
		SystemIdentityReader: stubSystemIdentityReader{values: identityfilewriter.SystemIdentityValues{
			SystemIdentity:     "",
			SystemIdentityPath: identityPath,
		}},
		NewWorker: identityfilewriter.NewWorker,
	}
	manifold := identityfilewriter.Manifold(cfg)

	w, err := manifold.Start(c.Context(), dt.StubGetter(nil))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.Not(tc.IsNil))
	w.Kill()
	c.Assert(w.Wait(), tc.ErrorIsNil)
}

// TestManifold_Start_InjectableWorker asserts that a custom NewWorker is
// called during Start.
func (s *ManifoldSuite) TestManifold_Start_InjectableWorker(c *tc.C) {
	dir := c.MkDir()
	identityPath := filepath.Join(dir, "system-identity")
	called := false
	cfg := identityfilewriter.ManifoldConfig{
		SystemIdentityReader: stubSystemIdentityReader{values: identityfilewriter.SystemIdentityValues{
			SystemIdentity:     "test-key",
			SystemIdentityPath: identityPath,
		}},
		NewWorker: func(cfg identityfilewriter.ManifoldConfig) (worker.Worker, error) {
			called = true
			return nil, nil
		},
	}
	manifold := identityfilewriter.Manifold(cfg)

	_, err := manifold.Start(c.Context(), dt.StubGetter(nil))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
}
