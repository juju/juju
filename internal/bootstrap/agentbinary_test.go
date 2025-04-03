// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/state/binarystorage"
)

type agentBinarySuite struct {
	baseSuite
}

var _ = gc.Suite(&agentBinarySuite{})

func (s *agentBinarySuite) TestPopulateAgentBinary(c *gc.C) {
	defer s.setupMocks(c).Finish()

	current := semversion.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}

	dir, toolsPath := s.ensureDirs(c, current)
	size := int64(4)

	s.writeDownloadTools(c, toolsPath, downloadTools{
		Version: current.String(),
		URL:     filepath.Join(dir, "tools", fmt.Sprintf("%s.tgz", current.String())),
		SHA256:  "sha256",
		Size:    size,
	})

	s.writeAgentBinary(c, toolsPath, current)

	s.storage.EXPECT().Add(gomock.Any(), gomock.Any(), binarystorage.Metadata{
		Version: current.String(),
		Size:    size,
		SHA256:  "sha256",
	}).Return(nil)

	s.agentBinaryStore.EXPECT().AddWithSHA256(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Arch:   current.Arch,
			Number: current.Number,
		},
		size,
		"sha256",
	).Return(nil)

	cleanup, err := PopulateAgentBinary(context.Background(), dir, s.storage, s.agentBinaryStore, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	cleanup()

	s.expectNoTools(c, toolsPath)
}

func (s *agentBinarySuite) TestPopulateAgentBinaryAddError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	current := semversion.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}

	dir, toolsPath := s.ensureDirs(c, current)
	size := int64(4)

	s.writeDownloadTools(c, toolsPath, downloadTools{
		Version: current.String(),
		URL:     filepath.Join(dir, "tools", fmt.Sprintf("%s.tgz", current.String())),
		SHA256:  "sha256",
		Size:    size,
	})

	s.writeAgentBinary(c, toolsPath, current)

	s.storage.EXPECT().Add(gomock.Any(), gomock.Any(), binarystorage.Metadata{
		Version: current.String(),
		Size:    size,
		SHA256:  "sha256",
	}).Return(errors.New("boom"))

	_, err := PopulateAgentBinary(context.Background(), dir, s.storage, s.agentBinaryStore, s.logger)
	c.Assert(err, gc.ErrorMatches, "boom")

	s.expectTools(c, toolsPath)
}

func (s *agentBinarySuite) TestPopulateAgentBinaryNoDownloadedToolsFile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	current := semversion.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}

	dir, _ := s.ensureDirs(c, current)

	_, err := PopulateAgentBinary(context.Background(), dir, s.storage, s.agentBinaryStore, s.logger)
	c.Assert(err, jc.ErrorIs, os.ErrNotExist)
}

func (s *agentBinarySuite) TestPopulateAgentBinaryNoBinaryFile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	current := semversion.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}

	dir, toolsPath := s.ensureDirs(c, current)
	size := int64(4)

	s.writeDownloadTools(c, toolsPath, downloadTools{
		Version: current.String(),
		URL:     filepath.Join(dir, "tools", fmt.Sprintf("%s.tgz", current.String())),
		SHA256:  "sha256",
		Size:    size,
	})

	_, err := PopulateAgentBinary(context.Background(), dir, s.storage, s.agentBinaryStore, s.logger)
	c.Assert(err, jc.ErrorIs, os.ErrNotExist)
}

func (s *agentBinarySuite) ensureDirs(c *gc.C, current semversion.Binary) (string, string) {
	dir := c.MkDir()

	path := filepath.Join(dir, "tools", current.String())

	err := os.MkdirAll(path, 0755)
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Stat(path)
	c.Assert(err, jc.ErrorIsNil)

	return dir, path
}

func (s *agentBinarySuite) writeDownloadTools(c *gc.C, dir string, tools downloadTools) {
	b, err := json.Marshal(tools)
	c.Assert(err, jc.ErrorIsNil)

	err = os.WriteFile(filepath.Join(dir, "downloaded-tools.txt"), b, 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *agentBinarySuite) writeAgentBinary(c *gc.C, dir string, current semversion.Binary) {
	err := os.WriteFile(filepath.Join(dir, "tools.tar.gz"), []byte("data"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s.sha256", current.String())), []byte("sha256"), 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *agentBinarySuite) expectNoTools(c *gc.C, dir string) {
	_, err := os.Stat(filepath.Join(dir, "tools.tar.gz"))
	c.Assert(err, jc.ErrorIs, os.ErrNotExist)
}

func (s *agentBinarySuite) expectTools(c *gc.C, dir string) {
	_, err := os.Stat(filepath.Join(dir, "tools.tar.gz"))
	c.Assert(err, jc.ErrorIsNil)
}

type downloadTools struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}
