// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"path/filepath"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"syscall"
)

// Patches for testing.
var (
	configDirName    = "/etc/network"
	configFileName   = filepath.Join(configDirName, "interfaces")
	configSubDirName = filepath.Join(configDirName, "interfaces.d")
)

// Indication what to do with file.
const (
	doNone = iota
	doWrite
	doRemove
)

// file struct contains information about config file, its contents and what to do to apply changes.
type ConfigFile struct {
	FileName string
	Data     string
	Op       int
}

// ConfigFiles contains information about all configuration files. Key for map is
// - empty string for /etc/netwotk/interfaces file
// - interface name for /etc/netwotk/interfaces.d/*.cfg files
// - "#" + filename for the other files
type ConfigFiles map[string]*ConfigFile

func (cf ConfigFiles) readOneFile(key, fileName string) error {
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		logger.Errorf("failed to read file %q: %v", fileName, err)
		return err
	}
	cf[key] = &ConfigFile{
		FileName: fileName,
		Data: string(data),
	}
	return nil
}

func (cf *ConfigFiles) readAll() error {
	var err error
	*cf = ConfigFiles{}
	err = cf.readOneFile("", configFileName)
	if err != nil {
		return err
	}

	// Read /etc/network/interfaces.d/*.cfg files.
	files, err := ioutil.ReadDir(configSubDirName)
	if err != nil {
		if e, ok := err.(*os.PathError); ok && e.Err == syscall.ENOENT {
			// ignore error when directory is absent
		} else {
			logger.Errorf("failed to read directory %q: %#v\n", configSubDirName, err)
			return err
		}
	}
	for _, info := range files {
		name := info.Name()
		if info.Mode().IsRegular() && len(name) > 4 && name[len(name)-4:] == ".cfg" {
			ifaceName := name[:len(name)-4]
			err = cf.readOneFile(ifaceName, fmt.Sprintf("%s/%s", configSubDirName, name))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (cf ConfigFiles) writeOrRemove() error {
	// Create /etc/network/interfaces.d directory is absent
	err := os.Mkdir(configSubDirName, 0755)
	if err != nil {
		if e, ok := err.(*os.PathError); ok && e.Err == syscall.EEXIST {
			// ignore error when directory already exists
		} else {
			logger.Errorf("failed to create directory %q: %#v\n", configSubDirName, err)
			return err
		}
	}
	for _, f := range cf {
		fileName := f.FileName
		if f.Op == doRemove {
			err = os.Remove(fileName)
			if err != nil {
				logger.Errorf("failed to remove file %q: %v", fileName, err)
				return err
			}
		} else if f.Op == doWrite {
			err := ioutil.WriteFile(fileName, []byte(f.Data), 0644)
			if err != nil {
				logger.Errorf("failed to white file %q: %v", fileName, err)
				return err
			}
		}
	}
	return nil
}

const managedPrefix = "# Managed by Networker, don't change\n"
const managedFormat = "%sauto %s\niface %s %s\n"

func managedText(ifaceName, configText string) string {
	return fmt.Sprintf(managedFormat, managedPrefix, ifaceName, ifaceName, configText)
}

func ifaceConfigFileName(ifaceName string) string {
	return filepath.Join(configSubDirName, ifaceName + ".cfg")
}

func (cf ConfigFiles) addManaged(ifaceName, configText string) {
	cf[ifaceName] = &ConfigFile{
		FileName: ifaceConfigFileName(ifaceName),
		Data:     managedText(ifaceName, configText),
		Op:       doWrite,
	}
}

func (cf ConfigFiles) removeManaged(ifaceName string) {
	if cf[ifaceName] != nil {
		cf[ifaceName].Data = ""
		cf[ifaceName].Op = doRemove
	}
}

func (cf ConfigFiles) isManaged(ifaceName string) bool {
	return ifaceName != privateInterface &&
		ifaceName != privateBridge &&
		cf[ifaceName] != nil &&
		len(cf[ifaceName].Data) > len(managedPrefix) &&
		cf[ifaceName].Data[:len(managedPrefix)] == managedPrefix
}

func (cf ConfigFiles) isChanged(ifaceName, configText string) bool {
	return ifaceName != privateInterface &&
		ifaceName != privateBridge &&
		(cf[ifaceName] == nil || cf[ifaceName].Data != managedText(ifaceName, configText))
}

// Filter out non-maintainable interfaces
func (cf ConfigFiles) filterManaged() {
	for key, file := range cf {
		if key == "" ||
			key[0] == '#' ||
			key == privateInterface ||
			key == privateBridge ||
			len(file.Data) <= len(managedPrefix) ||
			file.Data[:len(managedPrefix)] != managedPrefix {
			delete(cf, key)
		}
	}
}

func (cf *ConfigFiles) readManaged() error {
	if err := cf.readAll(); err != nil {
		return err
	}
	cf.filterManaged()
	return nil
}

func splitByInterfaces(data string) (map[string]string, error) {
	re, err := regexp.Compile(`(^|\n)(#[^\n]*\n)*(auto|allow\-\w+|iface|mapping|source|source\-directory)\s+([^\s]+)`)
	if err != nil {
		return nil, fmt.Errorf("should not be: %s", err)
	}
	result := make(map[string]string)
	pos := 0
	key := ""
	iii := re.FindAllStringSubmatchIndex(data, -1)
	for _, ii := range iii {
		value := data[pos:ii[3]]
		pos = ii[3]
		result[key] += value
		stanza := data[ii[6]:ii[7]]
		key = data[ii[8]:ii[9]]
		// source stanzas and local interface configurations remains in the main file
		if stanza == "source" || stanza == "source-directory" || key == "lo" {
			key = ""
		}
	}
	value := data[pos:]
	result[key] += value

	// Strip extra line feeds at the end of configurations
	re, err = regexp.Compile(`\n+$`)
	if err != nil {
		return nil, fmt.Errorf("should not be: %s", err)
	}
	for i, _ := range result {
		result[i] = re.ReplaceAllString(result[i], "\n")
	}
	return result, nil
}

const SourceCommentAndCommand = `# Source interfaces
# Please check %s before changing this file
# as interfaces may have been defined in %s
# NOTE: the primary ethernet device is defined in
# %s/eth0.cfg
# See LP: #1262951
source %s/*.cfg
`

func (cf ConfigFiles) fixMAAS() error {
	re, err := regexp.Compile(fmt.Sprintf("(^|\n)source\\s+(%s/[0-9A-Za-z_.:]+\\.config)\\s*\n",
		regexp.QuoteMeta(configDirName)))
	if err != nil {
		return fmt.Errorf("should not be: %s", err)
	}
	data := cf[""].Data
	for sl := re.FindStringSubmatchIndex(data); len(sl) == 6; sl = re.FindStringSubmatchIndex(data) {
		fileName := data[sl[4]:sl[5]]
		key := "#" + fileName
		err = cf.readOneFile(key, fileName)
		if err != nil {
			return err
		}
		data = data[:sl[3]] + cf[key].Data + data[sl[1]:]
		cf[""].Op = doWrite
		cf[key].Data = ""
		cf[key].Op = doRemove
	}

	// Verify the presence of source line to load files from /etc/network/interfaces.d
	re, err = regexp.Compile(fmt.Sprintf("(^|\n)source\\s+%s\\s*\n", regexp.QuoteMeta(configSubDirName+"/*.cfg")))
	if err != nil {
		return fmt.Errorf("should not be: %s", err)
	}
	if !re.MatchString(data) {
		// Should add source line and delete from files from /etc/network/interfaces.d,
		// because they were not intended to load
		data += fmt.Sprintf(SourceCommentAndCommand, configSubDirName, configSubDirName,
			configSubDirName, configSubDirName)
		for ifaceName, f := range cf {
			if ifaceName != "" && ifaceName[0] != '#' {
				f.Data = ""
				f.Op = doRemove
			}
		}
	}

	// Split /etc/network/interfaces into files in /etc/network/interfaces.d
	parts, err := splitByInterfaces(data)
	if err != nil {
		return err
	}
	if len(parts) != 1 {
		for ifaceName, part := range parts {
			var fileName string
			if ifaceName != "" {
				fileName = fmt.Sprintf("%s/%s.cfg", configSubDirName, ifaceName)
			} else {
				fileName = configFileName
			}
			cf[ifaceName] = &ConfigFile{
				FileName: fileName,
				Data:     part,
				Op:       doWrite,
			}
		}
	}
	return nil
}
