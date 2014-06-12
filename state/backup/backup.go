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

var logger = loggo.GetLogger("juju.backup")

// tarFiles creates a tar archive at targetPath holding the files listed
// in fileList. If compress is true, the archive will also be gzip
// compressed.
func tarFiles(fileList []string, targetPath, strip string, compress bool) (shaSum string, err error) {
	sha256hash := sha256.New()
	if err := tarAndHashFiles(fileList, targetPath, strip, compress, sha256hash); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256hash.Sum(nil)), nil
}

func tarAndHashFiles(fileList []string, targetPath, strip string, compress bool, hashw io.Writer) (err error) {
	checkClose := func(w io.Closer) {
		if closeErr := w.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("error closing backup file: %v", closeErr)
		}
	}
	f, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("cannot create backup file %q", targetPath)
	}
	defer checkClose(f)

	w := io.MultiWriter(f, hashw)

	if compress {
		gzw := gzip.NewWriter(w)
		defer checkClose(gzw)
		w = gzw
	}

	tarw := tar.NewWriter(w)
	defer checkClose(tarw)
	for _, ent := range fileList {
		if err := writeContents(ent, strip, tarw); err != nil {
			return fmt.Errorf("backup failed: %v", err)
		}
	}
	return nil
}

// writeContents creates an entry for the given file
// or directory in the given tar archive.
func writeContents(fileName, strip string, tarw *tar.Writer) error {
	fInfo, err := os.Stat(fileName)
	if err != nil {
		return err
	}
	h, err := tar.FileInfoHeader(fInfo, "")
	if err != nil {
		return fmt.Errorf("cannot create tar header for %q: %v", fileName, err)
	}
	h.Name = strings.TrimPrefix(fileName, strip)
	h.Name = strings.TrimPrefix(h.Name, string(os.PathSeparator))
	if err := tarw.WriteHeader(h); err != nil {
		return fmt.Errorf("cannot write header for %q: %v", fileName, err)
	}
	if fInfo.IsDir() {
		if !strings.HasSuffix(fileName, string(os.PathSeparator)) {
			fileName = fileName + string(os.PathSeparator)
		}
		directoryChildren, err := filepath.Glob(fileName + "*")
		if err != nil {
			fmt.Errorf("malformed directory listing expression: %q", fileName)
		}
		for _, directoryChild := range directoryChildren {
			err = writeContents(directoryChild, strip, tarw)
			if err != nil {
				return err
			}
		}
		return nil
	}
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(tarw, f); err != nil {
		return fmt.Errorf("failed to write %q: %v", fileName, err)
	}
	return nil
}

func _getFilesToBackup() ([]string, error) {

	LIBJUJU_PATH := "/var/lib/juju"
	initMachineConfs, err := filepath.Glob("/etc/init/jujud-machine-*.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch machine upstart files: %v", err)
	}
	agentConfs, err := filepath.Glob(path.Join(LIBJUJU_PATH, "agents", "machine-*"))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch agent configuration files: %v", err)
	}
	jujuLogConfs, err := filepath.Glob("/etc/rsyslog.d/*juju.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch juju log conf files: %v", err)
	}

	backupFiles := []string{
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
	backupFiles = append(backupFiles, initMachineConfs...)
	backupFiles = append(backupFiles, agentConfs...)
	backupFiles = append(backupFiles, jujuLogConfs...)
	return backupFiles, nil
}

var getFilesToBackup = _getFilesToBackup

func _runCommand(command string) (string, error) {
	dumpCommand := exec.Command(command)
	output, err := dumpCommand.Output()
	if err != nil {
		return "", fmt.Errorf("external command failed: %v", err)
	}
	return string(output), nil
}

var runCommand = _runCommand

func _getMongodumpPath() (string, error) {
	mongoDumpPath := "/usr/lib/juju/bin/mongodump"

	if _, err := os.Stat(mongoDumpPath); err == nil {
		return mongoDumpPath, nil
	}

	path, err := exec.LookPath("mongodump")
	if err != nil {
		logger.Infof("could not find %v or mongodump in $PATH", mongoDumpPath)
		return "", err
	}
	return path, nil

}

var getMongodumpPath = _getMongodumpPath

// Backup creates a tar.gz file named juju-backup_<date YYYYMMDDHHMMSS>.tar.gz
// in the specified outputFolder.
// The backup contains a dump folder with the output of mongodump command
// and a root.tar file which contains all the system files obtained from
// the output of getFilesToBackup
func Backup(adminPassword, outputFolder string, mongoPort int) (string, string, error) {
	// YYYYMMDDHHMMSS
	formattedDate := time.Now().Format("20060102150405")

	bkpFile := fmt.Sprintf("juju-backup_%s.tar.gz", formattedDate)

	mongodumpPath, err := getMongodumpPath()
	if err != nil {
		return "", "", fmt.Errorf("mongodump not available: %v", err)
	}

	tempDir, err := ioutil.TempDir("", "jujuBackup")
	defer os.RemoveAll(tempDir)
	bkpDir := path.Join(tempDir, "juju-backup")
	dumpDir := path.Join(bkpDir, "dump")
	err = os.MkdirAll(dumpDir, os.FileMode(0755))
	if err != nil {
		return "", "", fmt.Errorf("unable to create backup temporary directory: %v", err)
	}

	cmd := fmt.Sprintf("%s --oplog --ssl "+
		"--host localhost:%d "+
		"--username admin "+
		"--password '%s'"+
		" --out %s", mongodumpPath, mongoPort, adminPassword, dumpDir)
	_, err = runCommand(cmd)
	if err != nil {
		return "", "", fmt.Errorf("failed to dump database: %v", err)
	}

	tarFile := path.Join(bkpDir, "root.tar")
	backupFiles, err := getFilesToBackup()
	if err != nil {
		return "", "", fmt.Errorf("unable to determine files to backup: %v")
	}
	_, err = tarFiles(backupFiles, tarFile, "", false)
	if err != nil {
		return "", "", fmt.Errorf("unable to backup configuration files: %v", err)
	}

	shaSum, err := tarFiles([]string{bkpDir},
		path.Join(outputFolder, bkpFile),
		tempDir,
		true)
	if err != nil {
		return "", "", fmt.Errorf("unable to tar configuration files: %v", err)
	}
	return bkpFile, shaSum, nil
}
