// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/juju/utils"
)

var (
	configDirName    = "/etc/network"
	configFileName   = filepath.Join(configDirName, "interfaces")
	configSubDirName = filepath.Join(configDirName, "interfaces.d")
)

// Indication what to do with the interface config file in writeOrRemove method.
const (
	doNone = iota
	doWrite
	doRemove
)

// ConfigFile contains information about config file,
// its content and what to do to apply changes.
type ConfigFile struct {
	Data string
	Op   int
}

// ConfigFiles contains information about all configuration files.
// Map key is file name either /etc/netwotk/interfaces or /etc/netwotk/interfaces.d/*.cfg.
type ConfigFiles map[string]*ConfigFile

// readOneFile reads one file and stores it in the map cf with fileName as a key.
func (cf ConfigFiles) readOneFile(fileName string) error {
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		logger.Errorf("failed to read file %q: %v", fileName, err)
		return err
	}
	cf[fileName] = &ConfigFile{Data: string(data)}
	return nil
}

// readAll reads /etc/network/interfaces and /etc/network/interfaces.d/*.cfg files
func (cf *ConfigFiles) readAll() error {
	// Reset the map with configurations.
	*cf = ConfigFiles{}

	// Read the content of /etc/network/interfaces
	if err := cf.readOneFile(configFileName); err != nil {
		return err
	}

	// Check the presence of /etc/network/interfaces.d/ direcotory.
	fi, err := os.Stat(configSubDirName)
	if err == nil && fi.IsDir() {
		// Read the content of /etc/network/interfaces.d/ directory.
		files, err := ioutil.ReadDir(configSubDirName)
		if err != nil {
			logger.Errorf("failed to read directory %q: %v", configSubDirName, err)
			return err
		}
		// Read all /etc/network/interfaces.d/*.cfg files.
		for _, info := range files {
			if info.Mode().IsRegular() && filepath.Ext(info.Name()) == ".cfg" {
				err = cf.readOneFile(filepath.Join(configSubDirName, info.Name()))
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// writeOrRemove writes the new content of the file or removes it according to Op field.
func (cf ConfigFiles) writeOrRemove() error {
	// Create /etc/network/interfaces.d directory if absent.
	if _, err := os.Stat(configSubDirName); err != nil {
		err := os.Mkdir(configSubDirName, 0755)
		if err != nil {
			logger.Errorf("failed to create directory %q: %v", configSubDirName, err)
			return err
		}
	}

	for fileName, f := range cf {
		if f.Op == doRemove {
			err := os.Remove(fileName)
			if err != nil {
				logger.Errorf("failed to remove file %q: %v", fileName, err)
				return err
			}
		} else if f.Op == doWrite {
			err := utils.AtomicWriteFile(fileName, []byte(f.Data), 0644)
			if err != nil {
				logger.Errorf("failed to white file %q: %v", fileName, err)
				return err
			}
		}
	}
	return nil
}

// ifaceConfigFileName returns the fileName that stores the configuration for ifaceName.
func ifaceConfigFileName(ifaceName string) string {
	return filepath.Join(configSubDirName, ifaceName+".cfg")
}

// managedPrefix is the prefix that always presents in configuration file for interfaces managed by juju.
const managedPrefix = "# Managed by Juju, don't change.\n"

// addManaged makes ifaceName configureation to be written.
func (cf ConfigFiles) addManaged(ifaceName, configText string) {
	cf[ifaceConfigFileName(ifaceName)] = &ConfigFile{
		Data: managedPrefix + configText,
		Op:   doWrite,
	}
}

// removeManaged marks ifaceName configuration to be removed.
func (cf ConfigFiles) removeManaged(ifaceName string) {
	if cf[ifaceConfigFileName(ifaceName)] != nil {
		cf[ifaceName].Data = ""
		cf[ifaceName].Op = doRemove
	}
}

// isManaged checks whether the ifaceName is an interface managed by juju.
func (cf ConfigFiles) isManaged(ifaceName string) bool {
	fileName := ifaceConfigFileName(ifaceName)
	return ifaceName != privateInterface &&
		ifaceName != privateBridge &&
		cf[fileName] != nil &&
		len(cf[fileName].Data) > len(managedPrefix) &&
		cf[fileName].Data[:len(managedPrefix)] == managedPrefix
}

// isChanged checks whether the configuration text for ifaceName has changed.
func (cf ConfigFiles) isChanged(ifaceName, configText string) bool {
	return ifaceName != privateInterface &&
		ifaceName != privateBridge &&
		(cf[ifaceName] == nil || cf[ifaceName].Data != managedPrefix+configText)
}

// filterManaged filters out interfaces that are not managed by juju.
func (cf ConfigFiles) filterManaged() {
	for fileName, file := range cf {
		if fileName == configFileName ||
			fileName == ifaceConfigFileName(privateInterface) ||
			fileName == ifaceConfigFileName(privateBridge) ||
			len(file.Data) <= len(managedPrefix) ||
			file.Data[:len(managedPrefix)] != managedPrefix {
			delete(cf, fileName)
		}
	}
}

// readManaged only reads the configuration for interfaces managed by juju.
func (cf *ConfigFiles) readManaged() error {
	if err := cf.readAll(); err != nil {
		return err
	}
	cf.filterManaged()
	return nil
}

// splitRegExp is used to split /etc/interfaces/network by stanzas
// The below digits are indices of positions returned by regexp.FindAllStringSubmatchIndex.
var splitRegExp = regexp.MustCompile(
	`(^|\n)(#[^\n]*\n)*(auto|allow\-\w+|iface|mapping|source|source\-directory)\s+([^\s]+)`)

//       0                                                                                   1
//       2    34         5 6                                                      7   8      9

// splitByInterfaces splits /etc/network/interfaces content to separate configuration
// for each interace.
// Comment lines (started with '#') that directly precede a stanza belongs to the stanza.
func splitByInterfaces(data string) (map[string]string, error) {
	result := map[string]string{}
	pos := 0
	ifaceName := ""
	sliceOfIndices := splitRegExp.FindAllStringSubmatchIndex(data, -1)
	for _, indices := range sliceOfIndices {
		endOfPrevious := data[pos:indices[3]]
		result[ifaceName] += endOfPrevious
		pos = indices[3]
		stanza := data[indices[6]:indices[7]]
		ifaceName = data[indices[8]:indices[9]]
		// "source*" stanzas and local interface configuration will remain in /etc/network/interfaces.
		if stanza == "source" || stanza == "source-directory" || ifaceName == "lo" {
			ifaceName = ""
		}
	}
	result[ifaceName] += data[pos:]

	// Strip extra line feeds at end of configurations
	for i, _ := range result {
		result[i] = strings.TrimRight(result[i], "\n") + "\n"
	}
	return result, nil
}

// sourceCommentAndCommand is the text that present in default Ubuntu /etc/network/interfaces file
const sourceCommentAndCommand = `# Source interfaces
# Please check %s before changing this file
# as interfaces may have been defined in %s
# NOTE: the primary ethernet device is defined in
# %s/eth0.cfg
# See LP: #1262951
source %s/*.cfg
`

// fixMAAS restores /etc/network/interfaces file to the original format
// Ubuntu 14.04+ uses (i.e. one master interfaces file and one .cfg
// file per interface).
// With time, this function will be dropped.
func (cf ConfigFiles) fixMAAS() error {
	// Remove "source eth0.config" lines, created by MAAS provider.
	re, err := regexp.Compile(fmt.Sprintf("(^|\n)source\\s+(%s/[0-9A-Za-z_.:]+\\.config)\\s*\n",
		regexp.QuoteMeta(configDirName)))
	if err != nil {
		return fmt.Errorf("should not be: %s", err)
	}
	data := cf[configFileName].Data
	for sl := re.FindStringSubmatchIndex(data); len(sl) != 0; sl = re.FindStringSubmatchIndex(data) {
		fileName := data[sl[4]:sl[5]]
		if err = cf.readOneFile(fileName); err != nil {
			return err
		}
		// Update the main config file.
		cf[configFileName].Op = doWrite
		data = data[:sl[3]] + cf[fileName].Data + data[sl[1]:]

		// Mark included file /etc/network/eth0.config to remove.
		cf[fileName].Data = ""
		cf[fileName].Op = doRemove
	}

	// Verify the presence of line 'source /etc/network/interfaces.d/*.cfg'
	re, err = regexp.Compile(fmt.Sprintf("(^|\n)source\\s+%s\\s*\n",
		regexp.QuoteMeta(filepath.Join(configSubDirName, "*.cfg"))))
	if err != nil {
		return fmt.Errorf("should not be: %s", err)
	}
	if !re.MatchString(data) {
		// Should add source line and delete from files from /etc/network/interfaces.d,
		// because they were not intended to load
		data += fmt.Sprintf(sourceCommentAndCommand, configSubDirName, configSubDirName,
			configSubDirName, configSubDirName)
		for ifaceName, f := range cf {
			if ifaceName != "" && ifaceName[0] != '#' {
				f.Data = ""
				f.Op = doRemove
			}
		}
	}

	// Split /etc/network/interfaces into /etc/network/interfaces.d/*.cfg files.
	parts, err := splitByInterfaces(data)
	if err != nil {
		return err
	}
	if len(parts) != 1 {
		for ifaceName, part := range parts {
			var fileName string
			if ifaceName == "" {
				fileName = configFileName
			} else {
				fileName = ifaceConfigFileName(ifaceName)
			}
			cf[fileName] = &ConfigFile{
				Data: part,
				Op:   doWrite,
			}
		}
	}
	return nil
}
