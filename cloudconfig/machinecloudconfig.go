package cloudconfig

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/v3"
	"gopkg.in/yaml.v2"

	corebase "github.com/juju/juju/core/base"
	utilsos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/paths"
)

// InitReader describes methods for extracting machine provisioning config,
// and extracting keys from config based on properties sourced from
// "container-inherit-properties" in model config.
type InitReader interface {
	// GetInitConfig aggregates and returns provisioning data from the machine.
	GetInitConfig() (map[string]interface{}, error)

	// ExtractPropertiesFromConfig filters the input config data according to
	// the input list of properties and returns the result
	ExtractPropertiesFromConfig([]string, map[string]interface{}, loggo.Logger) map[string]interface{}
}

// MachineInitReaderConfig holds configuration values required by
// MachineInitReader to retrieve initialisation configuration for
// a single machine.
type MachineInitReaderConfig struct {
	// Base is the base of the machine.
	Base corebase.Base

	// CloudInitConfigDir is the directory where cloud configuration resides
	// on MAAS hosts.
	CloudInitConfigDir string

	// CloudInitInstanceConfigDir is the directory where cloud-init data for
	// the instance resides. Cloud-Init user-data supplied to Juju lives here.
	CloudInitInstanceConfigDir string

	// CurtinInstallConfigFile is the file containing initialisation config
	// written by Curtin.
	// Apt configuration for MAAS versions 2.5+ resides here.
	CurtinInstallConfigFile string
}

// MachineInitReader accesses Cloud-Init and Curtin configuration data,
// and extracts from it values for keys set in model configuration as
// "container-inherit-properties".
type MachineInitReader struct {
	config MachineInitReaderConfig
}

// NewMachineInitReader creates and returns a new MachineInitReader for the
// input os name.
func NewMachineInitReader(base corebase.Base) (InitReader, error) {
	osType := paths.OSType(base.OS)
	cfg := MachineInitReaderConfig{
		Base:                       base,
		CloudInitConfigDir:         paths.CloudInitCfgDir(osType),
		CloudInitInstanceConfigDir: paths.MachineCloudInitDir(osType),
		CurtinInstallConfigFile:    paths.CurtinInstallConfig(osType),
	}
	return NewMachineInitReaderFromConfig(cfg), nil
}

// NewMachineInitReaderFromConfig creates and returns a new MachineInitReader using
// the input configuration.
func NewMachineInitReaderFromConfig(cfg MachineInitReaderConfig) InitReader {
	return &MachineInitReader{config: cfg}
}

// GetInitConfig returns a map of configuration data used to provision the
// machine. It is sourced from both Cloud-Init and Curtin data.
func (r *MachineInitReader) GetInitConfig() (map[string]interface{}, error) {
	switch ostype.OSTypeForName(r.config.Base.OS) {
	case ostype.Ubuntu, ostype.CentOS:
		base, err := utilsos.HostBase()
		if err != nil || r.config.Base != base {
			logger.Debugf("not attempting to get init config for %s, base of machine and container differ", r.config.Base.DisplayString())
			return nil, nil
		}
	default:
		logger.Debugf("not attempting to get init config for %s container", r.config.Base.DisplayString())
		return nil, nil
	}

	machineCloudInitData, err := r.getMachineCloudCfgDirData()
	if err != nil {
		return nil, errors.Trace(err)
	}

	file := filepath.Join(r.config.CloudInitInstanceConfigDir, "vendor-data.txt")
	vendorData, err := r.unmarshallConfigFile(file)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for k, v := range vendorData {
		machineCloudInitData[k] = v
	}

	_, curtinData, err := fileAsConfigMap(r.config.CurtinInstallConfigFile)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for k, v := range curtinData {
		machineCloudInitData[k] = v
	}

	return machineCloudInitData, nil
}

// getMachineCloudCfgDirData returns a map of the combined machine's Cloud-Init
// cloud.cfg.d config files. Files are read in lexical order.
func (r *MachineInitReader) getMachineCloudCfgDirData() (map[string]interface{}, error) {
	dir := r.config.CloudInitConfigDir

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.Annotate(err, "determining files in CloudInitCfgDir for the machine")
	}
	sortedFiles := sortableDirEntries(files)
	sort.Sort(sortedFiles)

	cloudInit := make(map[string]interface{})
	for _, file := range files {
		name := file.Name()
		if !strings.HasSuffix(name, ".cfg") {
			continue
		}
		_, cloudCfgData, err := fileAsConfigMap(filepath.Join(dir, name))
		if err != nil {
			return nil, errors.Trace(err)
		}
		for k, v := range cloudCfgData {
			cloudInit[k] = v
		}
	}
	return cloudInit, nil
}

// unmarshallConfigFile reads the file at the input path,
// decompressing it if required, and converts the contents to a map of
// configuration key-values.
func (r *MachineInitReader) unmarshallConfigFile(file string) (map[string]interface{}, error) {
	raw, config, err := fileAsConfigMap(file)
	if err == nil {
		return config, nil
	}
	if !errors.IsNotValid(err) {
		return nil, errors.Trace(err)
	}

	// The data maybe be gzipped, base64 encoded, both, or neither.
	// If both, it has been gzipped, then base64 encoded.
	logger.Tracef("unmarshall failed (%s), file may be compressed", err.Error())

	zippedData, err := utils.Gunzip(raw)
	if err == nil {
		cfg, err := bytesAsConfigMap(zippedData)
		return cfg, errors.Trace(err)
	}
	logger.Tracef("Gunzip of %q failed (%s), maybe it is encoded", file, err)

	decodedData, err := base64.StdEncoding.DecodeString(string(raw))
	if err == nil {
		if buf, err := bytesAsConfigMap(decodedData); err == nil {
			return buf, nil
		}
	}
	logger.Tracef("Decoding of %q failed (%s), maybe it is encoded and gzipped", file, err)

	decodedZippedBuf, err := utils.Gunzip(decodedData)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot unmarshall or decompress %q", file)
	}

	cfg, err := bytesAsConfigMap(decodedZippedBuf)
	return cfg, errors.Trace(err)
}

// fileAsConfigMap reads the file at the input path and returns its contents as
// raw bytes, and if possible a map of config key-values.
func fileAsConfigMap(file string) ([]byte, map[string]interface{}, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "reading config from %q", file)
	}
	if len(raw) == 0 {
		return nil, nil, nil
	}

	cfg, err := bytesAsConfigMap(raw)
	if err != nil {
		return raw, cfg, errors.NotValidf("converting %q contents to map: %s", file, err.Error())
	}
	return raw, cfg, nil
}

// ExtractPropertiesFromConfig filters the input config based on the
// input properties and returns a map of cloud-init data.
func (r *MachineInitReader) ExtractPropertiesFromConfig(
	keys []string, cfg map[string]interface{}, log loggo.Logger,
) map[string]interface{} {
	foundDataMap := make(map[string]interface{})
	for _, k := range keys {
		key := strings.TrimSpace(k)
		switch key {
		case "apt-security", "apt-primary", "apt-sources", "apt-sources_list":
			if val, ok := cfg["apt"]; ok {
				for k, v := range nestedAptConfig(key, val, log) {
					// security, sources, and primary all nest under apt, ensure
					// we don't overwrite prior translated data.
					if apt, ok := foundDataMap["apt"].(map[string]interface{}); ok {
						apt[k] = v
					} else {
						foundDataMap["apt"] = map[string]interface{}{
							k: v,
						}
					}
				}
			} else {
				log.Debugf("%s not found in machine init data", key)
			}
		case "ca-certs":
			// No translation needed, ca-certs the same in both versions of Cloud-Init.
			if val, ok := cfg[key]; ok {
				foundDataMap[key] = val
			} else {
				log.Debugf("%s not found in machine init data", key)
			}
		}
	}
	return foundDataMap
}

func nestedAptConfig(key string, val interface{}, log loggo.Logger) map[string]interface{} {
	split := strings.Split(key, "-")
	secondary := split[1]

	for k, v := range interfaceToMapStringInterface(val) {
		if k == secondary {
			foundDataMap := make(map[string]interface{})
			foundDataMap[k] = v
			return foundDataMap
		}
	}

	log.Debugf("%s not found in machine init data", key)
	return nil
}

type sortableDirEntries []os.DirEntry

func (fil sortableDirEntries) Len() int {
	return len(fil)
}

func (fil sortableDirEntries) Less(i, j int) bool {
	return fil[i].Name() < fil[j].Name()
}

func (fil sortableDirEntries) Swap(i, j int) {
	fil[i], fil[j] = fil[j], fil[i]
}

func bytesAsConfigMap(raw []byte) (map[string]interface{}, error) {
	dataMap := make(map[string]interface{})
	err := yaml.Unmarshal(raw, &dataMap)
	return dataMap, errors.Trace(err)
}

func interfaceToMapStringInterface(in interface{}) map[string]interface{} {
	if inMap, ok := in.(map[interface{}]interface{}); ok {
		outMap := make(map[string]interface{}, len(inMap))
		for k, v := range inMap {
			if key, ok := k.(string); ok {
				outMap[key] = v
			}
		}
		return outMap
	}
	return nil
}
