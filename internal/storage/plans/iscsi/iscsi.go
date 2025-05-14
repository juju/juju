// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package iscsi

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/utils/v4/exec"

	"github.com/juju/juju/core/blockdevice"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/storage/plans/common"
)

var logger = internallogger.GetLogger("juju.storage.plans.iscsi")

const (
	// ISCSI_ERR_SESS_EXISTS is the error code open-iscsi returns if the
	// target is already logged in
	ISCSI_ERR_SESS_EXISTS = 15

	// ISCSI_ERR_NO_OBJS_FOUND is the error code open-iscsi returns if
	// no records/targets/sessions/portals are found to execute operation on
	ISCSI_ERR_NO_OBJS_FOUND = 21
)

var (
	sysfsBlock = "/sys/block"

	sysfsiSCSIHost    = "/sys/class/iscsi_host"
	sysfsiSCSISession = "/sys/class/iscsi_session"

	iscsiConfigFolder = "/etc/iscsi"
)

type iscsiPlan struct{}

func NewiSCSIPlan() common.Plan {
	return &iscsiPlan{}
}

func (i *iscsiPlan) AttachVolume(volumeInfo map[string]string) (blockdevice.BlockDevice, error) {
	plan, err := newiSCSIInfo(volumeInfo)
	if err != nil {
		return blockdevice.BlockDevice{}, errors.Trace(err)
	}
	return plan.attach()
}

func (i *iscsiPlan) DetachVolume(volumeInfo map[string]string) error {
	plan, err := newiSCSIInfo(volumeInfo)
	if err != nil {
		return errors.Trace(err)
	}
	return plan.detach()
}

type iscsiConnectionInfo struct {
	iqn        string
	address    string
	port       int
	chapSecret string
	chapUser   string
}

var runCommand = func(params []string) (*exec.ExecResponse, error) {
	cmd := strings.Join(params, " ")
	execParams := exec.RunParams{
		Commands: cmd,
	}
	resp, err := exec.RunCommands(execParams)

	return resp, err
}

func getHardwareInfo(name string) (blockdevice.BlockDevice, error) {
	cmd := []string{
		"udevadm", "info",
		"-q", "property",
		"--path", fmt.Sprintf("/block/%s", name),
	}

	result, err := runCommand(cmd)
	if err != nil {
		return blockdevice.BlockDevice{}, errors.Annotatef(err, "error running udevadm")
	}
	blockDevice := blockdevice.BlockDevice{
		DeviceName: name,
	}
	var busId, serialId string
	s := bufio.NewScanner(bytes.NewReader(result.Stdout))
	for s.Scan() {
		line := s.Text()
		if line == "" {
			continue
		}
		if strings.Contains(line, "") {
			continue
		}
		fields := strings.SplitN(line, "=", 2)
		if len(fields) != 2 {
			logger.Tracef(context.TODO(), "failed to parse line %s", line)
			continue
		}

		key := fields[0]
		value := fields[1]
		switch key {
		case "ID_WWN":
			blockDevice.WWN = value
		case "DEVLINKS":
			blockDevice.DeviceLinks = strings.Split(value, " ")
		case "ID_BUS":
			busId = value
		case "ID_SERIAL":
			serialId = value
		}
	}
	if busId != "" && serialId != "" {
		blockDevice.HardwareId = fmt.Sprintf("%s-%s", busId, serialId)
	}
	return blockDevice, nil
}

func newiSCSIInfo(info map[string]string) (*iscsiConnectionInfo, error) {
	var iqn, address, user, secret, port string
	var ok bool
	if iqn, ok = info["iqn"]; !ok {
		return nil, errors.Errorf("missing required field: iqn")
	}
	if address, ok = info["address"]; !ok {
		return nil, errors.Errorf("missing required field: address")
	}
	if port, ok = info["port"]; !ok {
		return nil, errors.Errorf("missing required field: port")
	}

	iPort, err := strconv.Atoi(port)
	if err != nil {
		return nil, errors.Errorf("invalid port: %v", port)
	}
	user = info["chap-user"]
	secret = info["chap-secret"]
	plan := &iscsiConnectionInfo{
		iqn:        iqn,
		address:    address,
		port:       iPort,
		chapSecret: secret,
		chapUser:   user,
	}
	return plan, nil
}

// sessionBase returns the iSCSI sysfs session folder
func (i *iscsiConnectionInfo) sessionBase(deviceName string) (string, error) {
	lnkPath := filepath.Join(sysfsBlock, deviceName)
	lnkRealPath, err := os.Readlink(lnkPath)
	if err != nil {
		return "", err
	}
	fullPath, err := filepath.Abs(filepath.Join(sysfsBlock, lnkRealPath))
	if err != nil {
		return "", err
	}
	segments := strings.SplitN(fullPath[1:], "/", -1)
	if len(segments) != 9 {
		// iscsi block devices look like:
		// /sys/devices/platform/host2/session1/target2:0:0/2:0:0:1/block/sda
		return "", errors.Errorf("not an iscsi device")
	}
	if _, err := os.Stat(filepath.Join(sysfsiSCSIHost, segments[3])); err != nil {
		return "", errors.Errorf("not an iscsi device")
	}
	sessionPath := filepath.Join(sysfsiSCSISession, segments[4])
	if _, err := os.Stat(sessionPath); err != nil {
		return "", errors.Errorf("session does not exits")
	}
	return sessionPath, nil
}

func (i *iscsiConnectionInfo) deviceName() (string, error) {
	items, err := os.ReadDir(sysfsBlock)
	if err != nil {
		return "", err
	}

	for _, val := range items {
		sessionBase, err := i.sessionBase(val.Name())
		if err != nil {
			logger.Tracef(context.TODO(), "failed to get session folder for device %s: %s", val.Name(), err)
			continue
		}
		tgtnameFile := filepath.Join(sessionBase, "targetname")
		if _, err := os.Stat(tgtnameFile); err != nil {
			logger.Tracef(context.TODO(), "%s was not found. Skipping", tgtnameFile)
			continue
		}
		tgtname, err := os.ReadFile(tgtnameFile)
		if err != nil {
			return "", err
		}
		trimmed := strings.TrimSuffix(string(tgtname), "\n")
		if trimmed == i.iqn {
			return val.Name(), nil
		}
	}
	return "", errors.NotFoundf("device for iqn %s not found", i.iqn)
}

func (i *iscsiConnectionInfo) portal() string {
	return fmt.Sprintf("%s:%d", i.address, i.port)
}

func (i *iscsiConnectionInfo) configFile() string {
	hostPortPair := fmt.Sprintf("%s,%d", i.address, i.port)
	return filepath.Join(iscsiConfigFolder, i.iqn, hostPortPair)
}

func (i *iscsiConnectionInfo) isNodeConfigured() bool {
	if _, err := os.Stat(i.configFile()); err != nil {
		return false
	}
	return true
}

// addTarget adds the iscsi target config
func (i *iscsiConnectionInfo) addTarget() error {
	newNodeParams := []string{
		"iscsiadm", "-m", "node",
		"-o", "new",
		"-T", i.iqn,
		"-p", i.portal()}
	result, err := runCommand(newNodeParams)
	if err != nil {
		return errors.Annotatef(err, "iscsiadm failed to add new node: %s", result.Stderr)
	}

	startupParams := []string{
		"iscsiadm", "-m", "node",
		"-o", "update",
		"-T", i.iqn,
		"-n", "node.startup",
		"-v", "automatic"}
	result, err = runCommand(startupParams)
	if err != nil {
		return errors.Annotatef(err, "iscsiadm failed to set startup mode: %s", result.Stderr)
	}

	if i.chapSecret != "" && i.chapUser != "" {
		authModeParams := []string{
			"iscsiadm", "-m", "node",
			"-o", "update",
			"-T", i.iqn,
			"-p", i.portal(),
			"-n", "node.session.auth.authmethod",
			"-v", "CHAP",
		}
		result, err = runCommand(authModeParams)
		if err != nil {
			return errors.Annotatef(err, "iscsiadm failed to set auth method: %s", result.Stderr)
		}
		usernameParams := []string{
			"iscsiadm", "-m", "node",
			"-o", "update",
			"-T", i.iqn,
			"-p", i.portal(),
			"-n", "node.session.auth.username",
			"-v", i.chapUser,
		}
		result, err = runCommand(usernameParams)
		if err != nil {
			return errors.Annotatef(err, "iscsiadm failed to set auth username: %s", result.Stderr)
		}
		passwordParams := []string{
			"iscsiadm", "-m", "node",
			"-o", "update",
			"-T", i.iqn,
			"-p", i.portal(),
			"-n", "node.session.auth.password",
			"-v", i.chapSecret,
		}
		result, err = runCommand(passwordParams)
		if err != nil {
			return errors.Annotatef(err, "iscsiadm failed to set auth password: %s", result.Stderr)
		}
	}
	return nil
}

func (i *iscsiConnectionInfo) login() error {
	if i.isNodeConfigured() == false {
		if err := i.addTarget(); err != nil {
			return errors.Trace(err)
		}
	}
	loginCmd := []string{
		"iscsiadm", "-m", "node",
		"-T", i.iqn,
		"-p", i.portal(),
		"-l",
	}
	result, err := runCommand(loginCmd)
	if err != nil {
		// test if error code is because we are already logged into this target
		if result.Code != ISCSI_ERR_SESS_EXISTS {
			return errors.Annotatef(err, "iscsiadm failed to log into target: %d", result.Code)
		}
	}
	return nil
}

func (i *iscsiConnectionInfo) logout() error {
	logoutCmd := []string{
		"iscsiadm", "-m", "node",
		"-T", i.iqn,
		"-p", i.portal(),
		"-u",
	}
	result, err := runCommand(logoutCmd)
	if err != nil {
		if result.Code != ISCSI_ERR_NO_OBJS_FOUND {
			return errors.Annotatef(err, "iscsiadm failed to logout of target: %d", result.Code)
		}
	}
	return nil
}

func (i *iscsiConnectionInfo) delete() error {
	deleteNodeCmd := []string{
		"iscsiadm", "-m", "node",
		"-o", "delete",
		"-T", i.iqn,
		"-p", i.portal(),
	}
	result, err := runCommand(deleteNodeCmd)
	if err != nil {
		if result.Code != ISCSI_ERR_NO_OBJS_FOUND {
			return errors.Annotatef(err, "iscsiadm failed to delete node: %d", result.Code)
		}
	}
	return nil
}

func (i *iscsiConnectionInfo) attach() (blockdevice.BlockDevice, error) {
	if err := i.addTarget(); err != nil {
		return blockdevice.BlockDevice{}, errors.Trace(err)
	}

	if err := i.login(); err != nil {
		return blockdevice.BlockDevice{}, errors.Trace(err)
	}
	// Wait for device to show up
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			_, err := i.deviceName()
			return err
		},
		Attempts: 20,
		Delay:    time.Second,
		Clock:    clock.WallClock,
	})
	if err != nil {
		return blockdevice.BlockDevice{}, errors.Trace(err)
	}

	devName, err := i.deviceName()
	if err != nil {
		return blockdevice.BlockDevice{}, errors.Trace(err)
	}
	return getHardwareInfo(devName)
}

func (i *iscsiConnectionInfo) detach() error {
	if err := i.logout(); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(i.delete())
}
