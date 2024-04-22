// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/ssh"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/sshinit"
)

//go:generate go run go.uber.org/mock/mockgen -package sshinit_test -destination sshclient_mock_test.go github.com/juju/utils/v3/ssh Client

type sshInitSuite struct{}

var _ = gc.Suite(&sshInitSuite{})

func (s *sshInitSuite) TestFileTransport(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	options := &ssh.Options{}
	sc := NewMockClient(ctrl)
	sc.EXPECT().Copy(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(args []string, optionsIn *ssh.Options) error {
		c.Check(args, gc.HasLen, 2)
		c.Check(args[0], gc.Matches, "/tmp.*/juju-.*-(?:foo|bar)")
		c.Check(args[1], gc.Matches, ":/tmp.*/juju-.*-(?:foo|bar)")
		c.Check(optionsIn, gc.Equals, options)
		data, err := os.ReadFile(args[0])
		if !c.Check(err, jc.ErrorIsNil) {
			return err
		}
		if strings.HasSuffix(args[0], "foo") {
			c.Check(data, jc.SameContents, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
		} else if strings.HasSuffix(args[0], "bar") {
			c.Check(data, jc.SameContents, []byte{9, 8, 7, 6, 5, 4, 3, 2, 1, 0})
		}
		return nil
	})

	ft := sshinit.NewFileTransporter(sshinit.ConfigureParams{
		Client:     sc,
		SSHOptions: options,
	})

	pathFoo := ft.SendBytes("foo", []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	pathBar := ft.SendBytes("bar", []byte{9, 8, 7, 6, 5, 4, 3, 2, 1, 0})
	c.Assert(pathFoo, gc.Matches, "/tmp.*/juju-.*-foo")
	c.Assert(pathBar, gc.Matches, "/tmp.*/juju-.*-bar")

	err := ft.Dispatch(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshInitSuite) TestFileTransportErrors(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	options := &ssh.Options{}
	sc := NewMockClient(ctrl)
	sc.EXPECT().Copy(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(args []string, optionsIn *ssh.Options) error {
		c.Check(args, gc.HasLen, 2)
		c.Check(args[0], gc.Matches, "/tmp.*/juju-.*-(?:foo|bar)")
		c.Check(args[1], gc.Matches, ":/tmp.*/juju-.*-(?:foo|bar)")
		c.Check(optionsIn, gc.Equals, options)
		data, err := os.ReadFile(args[0])
		if !c.Check(err, jc.ErrorIsNil) {
			return err
		}
		if strings.HasSuffix(args[0], "foo") {
			c.Check(data, jc.SameContents, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
		} else if strings.HasSuffix(args[0], "bar") {
			c.Check(data, jc.SameContents, []byte{9, 8, 7, 6, 5, 4, 3, 2, 1, 0})
			return fmt.Errorf("bar had some problems")
		}
		return nil
	})

	ft := sshinit.NewFileTransporter(sshinit.ConfigureParams{
		Client:     sc,
		SSHOptions: options,
	})

	pathFoo := ft.SendBytes("foo", []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	pathBar := ft.SendBytes("bar", []byte{9, 8, 7, 6, 5, 4, 3, 2, 1, 0})
	c.Assert(pathFoo, gc.Matches, "/tmp.*/juju-.*-foo")
	c.Assert(pathBar, gc.Matches, "/tmp.*/juju-.*-bar")

	err := ft.Dispatch(context.Background())
	c.Assert(err, gc.ErrorMatches, `failed scp-ing file /tmp.*/juju-.*-bar to :/tmp.*/juju-.*-bar: bar had some problems`)
}

func (s *sshInitSuite) TestFileTransportParallel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	options := &ssh.Options{}
	sc := NewMockClient(ctrl)
	sc.EXPECT().Copy(gomock.Any(), gomock.Any()).Times(1000).DoAndReturn(func(args []string, optionsIn *ssh.Options) error {
		c.Check(args, gc.HasLen, 2)
		c.Check(args[0], gc.Matches, "/tmp.*/juju-.*")
		c.Check(args[1], gc.Matches, ":/tmp.*/juju-.*")
		c.Check(optionsIn, gc.Equals, options)
		data, err := os.ReadFile(args[0])
		if !c.Check(err, jc.ErrorIsNil) {
			return err
		}
		c.Check(data, jc.SameContents, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
		time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
		return nil
	})

	ft := sshinit.NewFileTransporter(sshinit.ConfigureParams{
		Client:     sc,
		SSHOptions: options,
	})

	for i := 0; i < 1000; i++ {
		hint := fmt.Sprintf("hint-%d", i)
		p := ft.SendBytes(hint, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
		c.Assert(p, gc.Matches, "/tmp.*/juju-.*-"+hint)
	}

	err := ft.Dispatch(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}
