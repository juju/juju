// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"time"

	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backup"
	"github.com/juju/juju/version"
)

//---------------------------
// fakes

type BackupFaker struct {
	Info    *backup.BackupInfo
	URL     string
	Failure error
	Error   error
}

func NewBackupFaker(info *backup.BackupInfo, url string, failure error) *BackupFaker {
	faker := BackupFaker{
		Info:    info,
		URL:     url,
		Failure: failure,
	}
	return &faker
}

func (bf *BackupFaker) Backup(args BackupArgs) (BackupResult, error) {
	var result BackupResult

	if bf.Failure != nil {
		return result, bf.Failure
	}
	result.Info = *bf.Info
	result.URL = bf.URL
	return result, nil
}

func (bf *BackupFaker) Create(name string) (*backup.BackupInfo, string, error) {
	if bf.Failure != nil {
		return nil, "", bf.Failure
	}
	var info backup.BackupInfo
	info = *bf.Info
	if name != "" {
		info.Name = name
	}
	return &info, bf.URL, nil
}

//---------------------------
// test suite

type BackupSuite struct {
	testing.IsolationSuite

	Name      string
	Timestamp *time.Time
	CheckSum  string
	Size      int64
	Version   *version.Number

	Faker  BackupFaker
	Client apiClient
}

func (s *BackupSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.Name = "juju-backup.tar.gz"
	timestamp := time.Now().UTC()
	s.Timestamp = &timestamp
	s.CheckSum = "some SHA-1 hash"
	s.Size = 42
	version := version.Current.Number
	s.Version = &version
}

func (s *BackupSuite) Info() *backup.BackupInfo {
	info := backup.BackupInfo{
		Name:      s.Name,
		Timestamp: *s.Timestamp,
		CheckSum:  s.CheckSum,
		Size:      s.Size,
		Version:   *s.Version,
	}
	return &info
}

func (s *BackupSuite) SetFakeBackupClient() {
	newAPI := func(client apiClient) BackupAPI {
		return &s.Faker
	}
	s.PatchValue(newBackupClientAPI, newAPI)
}

func (s *BackupSuite) setFaker(i *backup.BackupInfo, u string, f, e error) {
	s.Faker = BackupFaker{i, u, f, e}
	newAPI := func(dbinfo *backup.DBConnInfo, stor backup.BackupStorage) BackupAPI {
		return &s.Faker
	}
	s.PatchValue(newBackupServerAPI, newAPI)
}

func (s *BackupSuite) SetSuccess(
	info *backup.BackupInfo, url string,
) *backup.BackupInfo {
	if info == nil {
		info = s.Info()
	}
	s.setFaker(info, url, nil, nil)
	return info
}

func (s *BackupSuite) SetFailure(msg string, args ...interface{}) {
	failure := fmt.Errorf(msg, args...)
	s.setFaker(nil, "", failure, nil)
}

func (s *BackupSuite) SetError(msg string, args ...interface{}) {
	err := fmt.Errorf(msg, args...)
	s.setFaker(nil, "", nil, err)
}

func (s *BackupSuite) send(c *gc.C, a Action, n string) (*BackupResult, error) {
	req := BackupArgs{a, n}
	result, err := s.Client.Backup(req)
	return &result, err
}

func (s *BackupSuite) SendSuccess(
	c *gc.C, action Action, name string,
) *BackupResult {
	res, err := s.send(c, action, name)
	c.Assert(err, gc.IsNil)
	return res
}

func (s *BackupSuite) SendError(c *gc.C, action Action, name string) error {
	result, err := s.send(c, action, name)
	if err == nil {
		c.Error(fmt.Sprintf("%v", result))
	}
	c.Assert(err, gc.NotNil)
	return err
}

func (s *BackupSuite) CheckSuccess(
	c *gc.C, result *BackupResult, info *backup.BackupInfo, name, url string, err error,
) bool {
	res := c.Check(err, gc.IsNil)
	res = c.Check(result.URL, gc.Equals, url) && res
	if name == "" {
		res = c.Check(result.Info, gc.DeepEquals, info) && res
	} else {
		var infoCopy backup.BackupInfo
		infoCopy = *info
		infoCopy.Name = name
		res = c.Check(result.Info, gc.DeepEquals, infoCopy) && res
	}
	return res
}

func (s *BackupSuite) CheckError(
	c *gc.C, result *BackupResult, err error, msg string,
) bool {
	res := c.Check(err, gc.ErrorMatches, msg)
	if !res {
		c.Check(result, gc.IsNil)
	}
	return res
}

func (s *BackupSuite) CheckAPIClient(c *gc.C, client apiClient) bool {
	if client == nil {
		client = s.Client
	}
	args := BackupArgs{Action: ActionNoop}

	info := s.SetSuccess(nil, "")
	result, err := client.Backup(args)
	res := c.Check(err, gc.IsNil)
	res = s.CheckSuccess(c, &result, info, "", "", err) && res

	s.SetError("exploded!")
	result, err = client.Backup(args)
	res = c.Check(err, gc.IsNil)
	res = s.CheckError(c, &result, err, "exploded!") && res

	s.SetFailure("failed!")
	result, err = client.Backup(args)
	res = c.Check(err, gc.IsNil)
	res = s.CheckError(c, &result, err, "failed!") && res

	return res
}
