// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package audit_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/audit"
	coretesting "github.com/juju/juju/testing"
)

type auditLogFileSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&auditLogFileSuite{})

func (s *auditLogFileSuite) TestLogging(c *gc.C) {
	dir := c.MkDir()
	sink := audit.NewLogFileSink(dir)

	modelUUID := coretesting.ModelTag.Id()
	t0 := time.Date(2015, time.June, 1, 23, 2, 1, 0, time.UTC)
	t1 := time.Date(2015, time.June, 1, 23, 2, 2, 0, time.UTC)

	err := sink(audit.AuditEntry{
		Timestamp:     t0,
		ModelUUID:     modelUUID,
		RemoteAddress: "10.0.0.1",
		OriginType:    "API",
		OriginName:    "user-admin",
		Operation:     "deploy",
		Data:          map[string]interface{}{"foo": "bar"},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = sink(audit.AuditEntry{
		Timestamp:     t1,
		ModelUUID:     modelUUID,
		RemoteAddress: "10.0.0.2",
		OriginType:    "API",
		OriginName:    "user-admin",
		Operation:     "status",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the audit log file was populated as expected
	logPath := filepath.Join(dir, "audit.log")
	logContents, err := ioutil.ReadFile(logPath)
	c.Assert(err, jc.ErrorIsNil)
	line0 := "2015-06-01 23:02:01," + modelUUID + ",10.0.0.1,user-admin,API,deploy,map[foo:bar]\n"
	line1 := "2015-06-01 23:02:02," + modelUUID + ",10.0.0.2,user-admin,API,status,map[]\n"
	c.Assert(string(logContents), gc.Equals, line0+line1)

	// Check the file mode is as expected. This doesn't work on
	// Windows (but this code is very unlikely to run on Windows so
	// it's ok).
	if runtime.GOOS != "windows" {
		info, err := os.Stat(logPath)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(info.Mode(), gc.Equals, os.FileMode(0600))
	}
}
