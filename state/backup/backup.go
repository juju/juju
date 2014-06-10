// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.backup.backup")

func WriteInto(source string, dest io.Writer) error {
	sourceF, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceF.Close()
	_, err = io.Copy(dest, sourceF)
	return err
}

func TarFiles(fileList []string, targetPath string) (string, error) {
	sha256hash := sha256.New()
	var fw io.WriteCloser
	var err error
	if strings.HasSuffix(targetPath, ".gz") || strings.HasSuffix(targetPath, ".tgz") {
		unCompressedFw, err := os.Create(targetPath)
		if err != nil {
			return "", fmt.Errorf("cannot create backup file: %q", targetPath)
		}
		defer unCompressedFw.Close()
		fw = gzip.NewWriter(io.MultiWriter(unCompressedFw, sha256hash))
	} else {
		fw, err = os.Create(targetPath)
		if err != nil {
			return "", fmt.Errorf("cannot create backup file: %q", targetPath)
		}
	}

	tarw := tar.NewWriter(fw)
	//defer tarw.Close()

	for _, ent := range fileList {
		fInfo, err := os.Stat(ent)
		if err != nil {
			return "", fmt.Errorf("unable to add entry %q: %q", ent, err)
		}
		h, err := tar.FileInfoHeader(fInfo, "")
		if err != nil {
			return "", fmt.Errorf("unable to create tar header for %q: %q", ent, err)
		}
		logger.Debugf("adding entry: %#v", h)
		// Remove leading slash
		h.Name = strings.TrimLeft(h.Name, string(os.PathSeparator))
		err = tarw.WriteHeader(h)
		if err != nil {
			return "", err
		}
		if !fInfo.Mode().IsDir() {
			if err := WriteInto(ent, tarw); err != nil {
				return "", err
			}

			if err := WriteInto(ent, sha256hash); err != nil {
				return "", err
			}
		}
	}
	err = tarw.Close()
	if err != nil {
		return "", fmt.Errorf("Error finishing file: %q", err)
	}
	err = fw.Close()
	if err != nil {
		return "", fmt.Errorf("Error finishing file: %q", err)
	}

	return fmt.Sprintf("%x", sha256hash.Sum(nil)), nil

}

func getFilesToBackup() ([]string, error) {

	LIBJUJU_PATH := "/var/lib/juju"
	initMachineConfs, err := filepath.Glob("/etc/init/jujud-machine-*.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch machine upstart files: %q", err)
	}
	agentConfs, err := filepath.Glob(path.Join(LIBJUJU_PATH, "agents", "machine-*"))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch agent configuration files: %q", err)
	}
	jujuLogConfs, err := filepath.Glob("/etc/rsyslog.d/*juju.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch juju log conf files: %q", err)
	}

	backup_files := []string{
		"/etc/init/juju-db.conf",
		path.Join(LIBJUJU_PATH, "tools"),
		path.Join(LIBJUJU_PATH, "server.pem"),
		path.Join(LIBJUJU_PATH, "system-identity"),
		path.Join(LIBJUJU_PATH, "nonce.txt"),
		path.Join(LIBJUJU_PATH, "shared-secret"),
		"~/.ssh/authorized_keys",
		"/var/log/juju/all-machines.log",
		"/var/log/juju/machine-0.log",
	}
	backup_files = append(backup_files, initMachineConfs...)
	backup_files = append(backup_files, agentConfs...)
	backup_files = append(backup_files, jujuLogConfs...)
	return backup_files, nil
}

var GetFilesToBackup = getFilesToBackup

func runCommand(command string) (string, error) {
	dumpCommand := exec.Command("bash", "-c", command)
	output, err := dumpCommand.Output()
	if err != nil {
		return "", fmt.Errorf("external command failed: %q", err)
	}
	return string(output), nil
}

var RunCommand = runCommand

func BackUp(adminPassword, outputFolder string, mongoPort int) (string, string, error) {
	// YYYYMMDDHHMMSS
	formattedDate := time.Now().Format("20060102150405")

	bkpFile := fmt.Sprintf("juju-backup_%s.tar.gz", formattedDate)

	//XXX (hduran-8) How do we find out this in a multi OS way?
	mongoDumpExecutable := "/usr/lib/juju/bin/mongodump"

	tempDir, err := ioutil.TempDir("", "jujuBackup")
	defer os.RemoveAll(tempDir)

	bkpDir := path.Join(tempDir, "juju-backup")

	if _, err := os.Stat(mongoDumpExecutable); err != nil {
		return "", "", fmt.Errorf("mongodump not available: %q", err)
	}

	dumpDir := path.Join(bkpDir, "dump")
	err = os.MkdirAll(dumpDir, os.FileMode(0755))

	if err != nil {
		return "", "", fmt.Errorf("unable to create backup temporary directory: %q", err)
	}

	cmd := fmt.Sprintf("%s --oplog --ssl "+
		"--host localhost:%d "+
		"--username admin "+
		"--password '%s'"+
		" --out %s", mongoDumpExecutable, mongoPort, adminPassword, dumpDir)
	_, err = RunCommand(cmd)
	if err != nil {
		return "", "", fmt.Errorf("failed to dump database: %q", err)
	}

	tarFile := path.Join(bkpDir, "root.tar")
	backup_files, err := GetFilesToBackup()
	if err != nil {
		return "", "", fmt.Errorf("unable to determine files to backup: %q")
	}
	_, err = TarFiles(backup_files, tarFile)
	if err != nil {
		return "", "", fmt.Errorf("unable to backup configuration files: %q", err)
	}

	shaSum, err := TarFiles([]string{bkpDir}, path.Join(outputFolder, bkpFile))
	if err != nil {
		return "", "", fmt.Errorf("unable to tar configuration files: %q", err)
	}

	shaFileName := fmt.Sprintf("juju-backup_%s.sha256", formattedDate)
	shaFile, err := os.Create(path.Join(outputFolder, shaFileName))
	if err != nil {
		return "", "", fmt.Errorf("failed to create checksum file: %q", err)
	}
	defer shaFile.Close()
	shaFile.WriteString(shaSum)

	return bkpFile, shaFileName, nil
}
