// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup_test

import (
	"fmt"
	"time"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backup"
)

func (b *BackupSuite) TestDefaultFilename(c *gc.C) {
	filename := backup.DefaultFilename(nil)

	// This is a sanity check that no one accidentally
	// (or accidentally maliciously) breaks the default filename format.
	c.Check(filename, gc.Matches, `jujubackup-\d{8}-\d{6}\..*`)
	// The most crucial part is that the suffix is .tar.gz.
	c.Check(filename, gc.Matches, `.*\.tar\.gz$`)
}

func (b *BackupSuite) TestDefaultFilenameDateFormat(c *gc.C) {
	filename := backup.DefaultFilename(nil)

	var Y, M, D, h, m, s int
	template := fmt.Sprintf(backup.FilenameTemplate, backup.TimestampFormat)
	_, err := fmt.Sscanf(filename, template, &Y, &M, &D, &h, &m, &s)
	c.Assert(err, gc.IsNil)

	timestamp := time.Date(Y, time.Month(M), D, h, m, s, 0, time.UTC)
	elapsed := int(time.Since(timestamp)) / int(time.Second)
	c.Check(elapsed < 10, gc.Equals, true)
}

func (b *BackupSuite) TestDefaultFilenameUnique(c *gc.C) {
	filename1 := backup.DefaultFilename(nil)
	time.Sleep(1 * time.Second)
	filename2 := backup.DefaultFilename(nil)

	c.Check(filename1, gc.Not(gc.Equals), filename2)
}
