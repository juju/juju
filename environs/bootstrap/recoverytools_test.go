// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
)

// recoveryArchiveWithAgent creates the portion of a backup needed by the
// archived-tools fallback. Archive preflight is tested separately.
func recoveryArchiveWithAgent(c *tc.C, version semversion.Binary) (string, string) {
	var root bytes.Buffer
	tw := tar.NewWriter(&root)
	contents := []byte("archived agent")
	c.Assert(tw.WriteHeader(&tar.Header{
		Name: path.Join("var/lib/juju/tools", version.String(), "jujuagentd"),
		Mode: 0755, Typeflag: tar.TypeReg, Size: int64(len(contents)),
	}), tc.ErrorIsNil)
	_, err := tw.Write(contents)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tw.Close(), tc.ErrorIsNil)

	var backup bytes.Buffer
	gz := gzip.NewWriter(&backup)
	tw = tar.NewWriter(gz)
	c.Assert(tw.WriteHeader(&tar.Header{
		Name: "juju-backup/root.tar", Mode: 0600, Typeflag: tar.TypeReg,
		Size: int64(root.Len()),
	}), tc.ErrorIsNil)
	_, err = tw.Write(root.Bytes())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tw.Close(), tc.ErrorIsNil)
	c.Assert(gz.Close(), tc.ErrorIsNil)

	filename := filepath.Join(c.MkDir(), "backup.tar.gz")
	c.Assert(os.WriteFile(filename, backup.Bytes(), 0600), tc.ErrorIsNil)
	sum := sha256.Sum256(backup.Bytes())
	return filename, "sha256-" + hex.EncodeToString(sum[:])
}

func readAgentFromBundle(c *tc.C, filename string) string {
	file, err := os.Open(filename)
	c.Assert(err, tc.ErrorIsNil)
	defer file.Close()
	gz, err := gzip.NewReader(file)
	c.Assert(err, tc.ErrorIsNil)
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		c.Assert(err, tc.ErrorIsNil)
		if header.Name == "jujuagentd" {
			data, err := io.ReadAll(tr)
			c.Assert(err, tc.ErrorIsNil)
			return string(data)
		}
	}
	c.Fatalf("agent bundle has no jujuagentd")
	return ""
}
