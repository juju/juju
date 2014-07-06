// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/juju/loggo"
)

const (
	TimestampFormat  = "%04d%02d%02d-%02d%02d%02d" // YYYYMMDD-hhmmss
	FilenameTemplate = "jujubackup-%s.tar.gz"      // Takes the timestamp.
	CompressionType  = "application/x-tar-gz"
)

var logger = loggo.GetLogger("juju.backup")

//---------------------------
// timestamps and filenames

// DefaultTimestamp returns "now" as a string formatted as
// "YYYYMMDD-hhmmss".
func DefaultTimestamp(now time.Time) string {
	// Unfortunately time.Time.Format() is not smart enough for us.
	Y, M, D := now.Date()
	h, m, s := now.Clock()
	return fmt.Sprintf(TimestampFormat, Y, M, D, h, m, s)
}

// DefaultFilename returns a filename to use for a backup.  The name is
// derived from the current time and date.
func DefaultFilename() string {
	formattedDate := DefaultTimestamp(time.Now().UTC())
	return fmt.Sprintf(FilenameTemplate, formattedDate)
}

// TimestampFromDefaultFilename extracts the timestamp from the filename.
func TimestampFromDefaultFilename(filename string) (time.Time, error) {
	// Unfortunately we can't just use time.Parse().
	re, err := regexp.Compile(`-\d{8}-\d{6}\.`)
	if err != nil {
		return time.Time{}, err
	}
	match := re.FindString(filename)
	match = match[1:len(match)]

	var Y, M, D, h, m, s int
	fmt.Sscanf(match, TimestampFormat, &Y, &M, &D, &h, &m, &s)
	return time.Date(Y, time.Month(M), D, h, m, s, 0, time.UTC), nil
}

//---------------------------
// file hashes

// GetHash returns the SHA1 hash generated from the provided file.
func GetHash(file io.Reader) (string, error) {
	shahash := sha1.New()
	_, err := io.Copy(shahash, file)
	if err != nil {
		return "", fmt.Errorf("unable to extract hash: %v", err)
	}

	return base64.StdEncoding.EncodeToString(shahash.Sum(nil)), nil
}

// GetHash returns the SHA1 hash generated from the provided compressed file.
func GetHashUncompressed(compressed io.Reader, mimetype string) (string, error) {
	var archive io.ReadCloser
	var err error

	switch mimetype {
	case "application/x-tar":
		return "", fmt.Errorf("not compressed: %s", mimetype)
	default:
		// Fall back to "application/x-tar-gz".
		archive, err = gzip.NewReader(compressed)
	}
	if err != nil {
		return "", fmt.Errorf("unable to open: %v", err)
	}
	defer archive.Close()

	return GetHash(archive)
}

// GetHashByFilename opens the file, unpacks it if compressed, and
// computes the SHA1 hash of the contents.
func GetHashDefault(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("unable to open: %v", err)
	}
	defer file.Close()
	return GetHashUncompressed(file, CompressionType)
}

// GetHashByFilename opens the file, unpacks it if compressed, and
// computes the SHA1 hash of the contents.
func GetHashByFilename(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("unable to open: %v", err)
	}
	defer file.Close()

	var mimetype string
	if strings.HasSuffix(filename, ".tar.gz") {
		mimetype = "application/x-tar-gz"
	} else {
		ext := filepath.Ext(filename)
		mimetype = mime.TypeByExtension(ext)
		if mimetype == "" {
			return "", fmt.Errorf("unsupported filename (%s)", filename)
		}
	}

	if mimetype == "application/x-tar" {
		return GetHash(file)
	} else {
		return GetHashUncompressed(file, mimetype)
	}
}

//---------------------------
// archive helpers

func writeArchive(fileList []string, outfile io.Writer, strip string, compress bool) error {
	var err error

	if compress {
		gzw := gzip.NewWriter(outfile)
		defer gzw.Close()
		outfile = gzw
	}
	tarw := tar.NewWriter(outfile)
	defer tarw.Close()

	for _, ent := range fileList {
		if err = AddToTarfile(ent, strip, tarw); err != nil {
			return fmt.Errorf("backup failed: %v", err)
		}
	}

	return nil
}

// CreateArchive returns a sha1 hash of targetPath after writing out the
// archive.  This archive holds the files listed in fileList. If
// compress is true, the archive will also be gzip compressed.
func CreateArchive(fileList []string, targetPath, strip string, compress bool) (string, error) {
	// Create the archive file.
	tarball, err := os.Create(targetPath)
	if err != nil {
		return "", fmt.Errorf("cannot create backup file %q", targetPath)
	}
	defer tarball.Close()

	// Write out the archive.
	err = writeArchive(fileList, tarball, strip, compress)
	if err != nil {
		return "", err
	}

	// Return the hash
	tarball.Seek(0, os.SEEK_SET)
	var data []byte
	tarball.Read(data)
	tarball.Seek(0, os.SEEK_SET)
	if compress {
		// We want the hash of the uncompressed file, since the
		// compressed archive will have a different hash depending on
		// the compression format.
		//	    return GetHash(tarball)
		return GetHashUncompressed(tarball, CompressionType)
	} else {
		return GetHash(tarball)
	}
}

// AddToTarfile creates an entry for the given file
// or directory in the given tar archive.
func AddToTarfile(fileName, strip string, tarw *tar.Writer) error {
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	fInfo, err := f.Stat()
	if err != nil {
		return err
	}
	h, err := tar.FileInfoHeader(fInfo, "")
	if err != nil {
		return fmt.Errorf("cannot create tar header for %q: %v", fileName, err)
	}
	h.Name = filepath.ToSlash(strings.TrimPrefix(fileName, strip))
	if err := tarw.WriteHeader(h); err != nil {
		return fmt.Errorf("cannot write header for %q: %v", fileName, err)
	}
	if !fInfo.IsDir() {
		if _, err := io.Copy(tarw, f); err != nil {
			return fmt.Errorf("failed to write %q: %v", fileName, err)
		}
		return nil
	}
	if !strings.HasSuffix(fileName, string(os.PathSeparator)) {
		fileName = fileName + string(os.PathSeparator)
	}

	for {
		names, err := f.Readdirnames(100)
		if len(names) == 0 && err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("error reading directory %q: %v", fileName, err)
		}
		for _, name := range names {
			if err := AddToTarfile(filepath.Join(fileName, name), strip, tarw); err != nil {
				return err
			}
		}
	}

}

//---------------------------
// state backups

var getFilesToBackup = _getFilesToBackup

func _getFilesToBackup() ([]string, error) {
	const dataDir string = "/var/lib/juju"
	initMachineConfs, err := filepath.Glob("/etc/init/jujud-machine-*.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch machine upstart files: %v", err)
	}
	agentConfs, err := filepath.Glob(filepath.Join(dataDir, "agents", "machine-*"))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch agent configuration files: %v", err)
	}
	jujuLogConfs, err := filepath.Glob("/etc/rsyslog.d/*juju.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch juju log conf files: %v", err)
	}

	backupFiles := []string{
		"/etc/init/juju-db.conf",
		filepath.Join(dataDir, "tools"),
		filepath.Join(dataDir, "server.pem"),
		filepath.Join(dataDir, "system-identity"),
		filepath.Join(dataDir, "nonce.txt"),
		filepath.Join(dataDir, "shared-secret"),
		"/home/ubuntu/.ssh/authorized_keys",
		"/var/log/juju/all-machines.log",
		"/var/log/juju/machine-0.log",
	}
	backupFiles = append(backupFiles, initMachineConfs...)
	backupFiles = append(backupFiles, agentConfs...)
	backupFiles = append(backupFiles, jujuLogConfs...)
	return backupFiles, nil
}

var runCommand = _runCommand

func _runCommand(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	out, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	if _, ok := err.(*exec.ExitError); ok && len(out) > 0 {
		return fmt.Errorf("error executing %q: %s", cmd, strings.Replace(string(out), "\n", "; ", -1))
	}
	return fmt.Errorf("cannot execute %q: %v", cmd, err)
}

var getMongodumpPath = _getMongodumpPath

func _getMongodumpPath() (string, error) {
	const mongoDumpPath string = "/usr/lib/juju/bin/mongodump"

	if _, err := os.Stat(mongoDumpPath); err == nil {
		return mongoDumpPath, nil
	}

	path, err := exec.LookPath("mongodump")
	if err != nil {
		return "", err
	}
	return path, nil
}

// Backup creates a tar.gz file named juju-backup_<date YYYYMMDDHHMMSS>.tar.gz
// in the specified outputFolder.
// The backup contains a dump folder with the output of mongodump command
// and a root.tar file which contains all the system files obtained from
// the output of getFilesToBackup
func Backup(password string, username string, outputFolder string, addr string) (string, string, error) {
	bkpFile := filepath.Join(outputFolder, DefaultFilename())

	mongodumpPath, err := getMongodumpPath()
	if err != nil {
		return "", "", fmt.Errorf("mongodump not available: %v", err)
	}

	tempDir, err := ioutil.TempDir("", "jujuBackup-")
	defer os.RemoveAll(tempDir)
	bkpDir := filepath.Join(tempDir, "juju-backup")
	dumpDir := filepath.Join(bkpDir, "dump")
	err = os.MkdirAll(dumpDir, os.FileMode(0755))
	if err != nil {
		return "", "", fmt.Errorf("cannot create backup temporary directory: %v", err)
	}

	err = runCommand(
		mongodumpPath,
		"--oplog",
		"--ssl",
		"--host", addr,
		"--username", username,
		"--password", password,
		"--out", dumpDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to dump database: %v", err)
	}

	tarFile := filepath.Join(bkpDir, "root.tar")
	backupFiles, err := getFilesToBackup()
	if err != nil {
		return "", "", fmt.Errorf("cannot determine files to backup: %v", err)
	}
	_, err = CreateArchive(backupFiles, tarFile, string(os.PathSeparator), false)
	if err != nil {
		return "", "", fmt.Errorf("cannot backup configuration files: %v", err)
	}

	shaSum, err := CreateArchive([]string{bkpDir},
		bkpFile,
		fmt.Sprintf("%s%s", tempDir, string(os.PathSeparator)),
		true)
	if err != nil {
		return "", "", fmt.Errorf("cannot tar configuration files: %v", err)
	}
	return bkpFile, shaSum, nil
}
