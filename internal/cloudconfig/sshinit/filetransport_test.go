// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit_test

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/utils/v4/ssh"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/cloudconfig/sshinit"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package sshinit_test -destination sshclient_mock_test.go github.com/juju/utils/v4/ssh Client

type sshInitSuite struct{}

func TestSshInitSuite(t *stdtesting.T) {
	tc.Run(t, &sshInitSuite{})
}

func (s *sshInitSuite) TestFileTransport(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	options := &ssh.Options{}
	sc := NewMockClient(ctrl)
	sc.EXPECT().Copy(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(args []string, optionsIn *ssh.Options) error {
		c.Check(args, tc.HasLen, 2)
		c.Check(args[0], tc.Matches, "/tmp.*/juju-.*-(?:foo|bar)")
		c.Check(args[1], tc.Matches, ":/tmp.*/juju-.*-(?:foo|bar)")
		c.Check(optionsIn, tc.Equals, options)
		data, err := os.ReadFile(args[0])
		if !c.Check(err, tc.ErrorIsNil) {
			return err
		}
		if strings.HasSuffix(args[0], "foo") {
			c.Check(data, tc.SameContents, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
		} else if strings.HasSuffix(args[0], "bar") {
			c.Check(data, tc.SameContents, []byte{9, 8, 7, 6, 5, 4, 3, 2, 1, 0})
		}
		return nil
	})

	ft := sshinit.NewFileTransporter(sshinit.ConfigureParams{
		Client:     sc,
		SSHOptions: options,
	})

	pathFoo := ft.SendBytes("foo", []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	pathBar := ft.SendBytes("bar", []byte{9, 8, 7, 6, 5, 4, 3, 2, 1, 0})
	c.Assert(pathFoo, tc.Matches, "/tmp.*/juju-.*-foo")
	c.Assert(pathBar, tc.Matches, "/tmp.*/juju-.*-bar")

	err := ft.Dispatch(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *sshInitSuite) TestFileTransportErrors(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	options := &ssh.Options{}
	sc := NewMockClient(ctrl)
	sc.EXPECT().Copy(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(args []string, optionsIn *ssh.Options) error {
		c.Check(args, tc.HasLen, 2)
		c.Check(args[0], tc.Matches, "/tmp.*/juju-.*-(?:foo|bar)")
		c.Check(args[1], tc.Matches, ":/tmp.*/juju-.*-(?:foo|bar)")
		c.Check(optionsIn, tc.Equals, options)
		data, err := os.ReadFile(args[0])
		if !c.Check(err, tc.ErrorIsNil) {
			return err
		}
		if strings.HasSuffix(args[0], "foo") {
			c.Check(data, tc.SameContents, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
		} else if strings.HasSuffix(args[0], "bar") {
			c.Check(data, tc.SameContents, []byte{9, 8, 7, 6, 5, 4, 3, 2, 1, 0})
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
	c.Assert(pathFoo, tc.Matches, "/tmp.*/juju-.*-foo")
	c.Assert(pathBar, tc.Matches, "/tmp.*/juju-.*-bar")

	err := ft.Dispatch(c.Context())
	c.Assert(err, tc.ErrorMatches, `failed scp-ing file /tmp.*/juju-.*-bar to :/tmp.*/juju-.*-bar: bar had some problems`)
}

func (s *sshInitSuite) TestFileTransportParallel(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	options := &ssh.Options{}
	sc := NewMockClient(ctrl)
	sc.EXPECT().Copy(gomock.Any(), gomock.Any()).Times(1000).DoAndReturn(func(args []string, optionsIn *ssh.Options) error {
		c.Check(args, tc.HasLen, 2)
		c.Check(args[0], tc.Matches, "/tmp.*/juju-.*")
		c.Check(args[1], tc.Matches, ":/tmp.*/juju-.*")
		c.Check(optionsIn, tc.Equals, options)
		data, err := os.ReadFile(args[0])
		if !c.Check(err, tc.ErrorIsNil) {
			return err
		}
		c.Check(data, tc.SameContents, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
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
		c.Assert(p, tc.Matches, "/tmp.*/juju-.*-"+hint)
	}

	err := ft.Dispatch(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}
