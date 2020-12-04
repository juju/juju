package cloudconfig

import (
	"encoding/base64"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	utilsos "github.com/juju/os/v2"
	utilsseries "github.com/juju/os/v2/series"
	"github.com/juju/utils/v2"
	"gopkg.in/yaml.v2"

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
	// Series is the OS series of the machine.
	Series string

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
// input series.
func NewMachineInitReader(series string) (InitReader, error) {
	osType := paths.SeriesToOS(series)
	cfg := MachineInitReaderConfig{
		Series:                     series,
		CloudInitConfigDir:         paths.CloudInitCfgDir(osType),
		CloudInitInstanceConfigDir: paths.MachineCloudInitDir(osType),
		CurtinInstallConfigFile:    paths.CurtinInstallConfig(osType),
	}
	return NewMachineInitReaderFromConfig(cfg), nil
}

// NewMachineInitReader creates and returns a new MachineInitReader using
// the input configuration.
func NewMachineInitReaderFromConfig(cfg MachineInitReaderConfig) InitReader {
	return &MachineInitReader{config: cfg}
}

// GetInitConfig returns a map of configuration data used to provision the
// machine. It is sourced from both Cloud-Init and Curtin data.
func (r *MachineInitReader) GetInitConfig() (map[string]interface{}, error) {
	series := r.config.Series

	containerOS, err := utilsseries.GetOSFromSeries(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	switch containerOS {
	case utilsos.Ubuntu, utilsos.CentOS, utilsos.OpenSUSE:
		hostSeries, err := utilsseries.HostSeries()
		if err != nil || series != hostSeries {
			logger.Debugf("not attempting to get init config for %s, series of machine and container differ", series)
			return nil, nil
		}
	default:
		logger.Debugf("not attempting to get init config for %s container", series)
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

func (r *MachineInitReader) ExtractPropertiesFromConfig(
	keys []string, cfg map[string]interface{}, log loggo.Logger,
) map[string]interface{} {
	if r.config.Series == "trusty" {
		return extractPropertiesFromConfigLegacy(keys, cfg, log)
	}

	// There is a big assumption that supported CentOS and OpenSUSE versions
	// supported by juju are using cloud-init version >= 0.7.8
	return extractPropertiesFromConfig(keys, cfg, log)
}

// getMachineCloudCfgDirData returns a map of the combined machine's Cloud-Init
// cloud.cfg.d config files. Files are read in lexical order.
func (r *MachineInitReader) getMachineCloudCfgDirData() (map[string]interface{}, error) {
	dir := r.config.CloudInitConfigDir

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, errors.Annotate(err, "determining files in CloudInitCfgDir for the machine")
	}
	sortedFiles := sortableFileInfos(files)
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
		// During testing, it was found that the trusty vendor-data.txt.i file
		// can contain only the text "NONE", which doesn't unmarshall or decompress
		// we don't want to fail in that case.
		if r.config.Series == "trusty" {
			logger.Debugf("failed to unmarshall or decompress %q: %s", file, err)
			return nil, nil
		}
		return nil, errors.Annotatef(err, "cannot unmarshall or decompress %q", file)
	}

	cfg, err := bytesAsConfigMap(decodedZippedBuf)
	return cfg, errors.Trace(err)
}

// fileAsConfigMap reads the file at the input path and returns its contents as
// raw bytes, and if possible a map of config key-values.
func fileAsConfigMap(file string) ([]byte, map[string]interface{}, error) {
	raw, err := ioutil.ReadFile(file)
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

// extractPropertiesFromConfig filters the input config based on the
// input properties and returns a map of cloud-init data, compatible with
// version 0.7.8 and above.
func extractPropertiesFromConfig(props []string, cfg map[string]interface{}, log loggo.Logger) map[string]interface{} {
	foundDataMap := make(map[string]interface{})
	for _, k := range props {
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

// extractPropertiesFromConfigLegacy filters the input config based on the
// input properties and returns a map of cloud-init data, compatible with
// version 0.7.7 and below.
func extractPropertiesFromConfigLegacy(
	props []string, cfg map[string]interface{}, log loggo.Logger,
) map[string]interface{} {
	foundDataMap := make(map[string]interface{})
	aptProcessed := false

	for _, k := range props {
		key := strings.TrimSpace(k)
		switch key {
		case "apt-primary", "apt-sources":
			if aptProcessed {
				continue
			}
			for _, aptKey := range []string{"apt_mirror", "apt_mirror_search", "apt_mirror_search_dns", "apt_sources"} {
				if val, ok := cfg[aptKey]; ok {
					foundDataMap[aptKey] = val
				} else {
					log.Debugf("%s not found in machine init data", aptKey)
				}
			}
			aptProcessed = true
		case "apt-sources_list":
			// Testing series trusty on MAAS 2.5+ shows that this could be
			// treated in the same way as the non-legacy property
			// extraction, but we would then be mixing techniques.
			// Legacy handling is left unchanged here under the assumption
			// that provisioning trusty machines on much newer MAAS
			// versions is highly unlikely.
			log.Debugf("%q ignored for this machine series", key)
		case "apt-security":
			// Translation for apt-security unknown at this time.
			log.Debugf("%q ignored for this machine series", key)
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

type sortableFileInfos []os.FileInfo

func (fil sortableFileInfos) Len() int {
	return len(fil)
}

func (fil sortableFileInfos) Less(i, j int) bool {
	return fil[i].Name() < fil[j].Name()
}

func (fil sortableFileInfos) Swap(i, j int) {
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
