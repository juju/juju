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
	utilsos "github.com/juju/os"
	utilsseries "github.com/juju/os/series"
	"github.com/juju/utils"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/juju/paths"
)

// GetMachineCloudInitData returns a map of all cloud init data on the machine.
func GetMachineCloudInitData(series string) (map[string]interface{}, error) {
	containerOS, err := utilsseries.GetOSFromSeries(series)
	if err != nil {
		return nil, err
	}
	switch containerOS {
	case utilsos.Ubuntu, utilsos.CentOS, utilsos.OpenSUSE:
		if series != utilsseries.MustHostSeries() {
			logger.Debugf("not attempting to get cloudinit data for %s, series of machine and container differ", series)
			return nil, nil
		}
	default:
		logger.Debugf("not attempting to get cloudinit data for %s container", series)
		return nil, nil
	}

	machineCloudInitData, err := getMachineCloudCfgDirData(series)
	if err != nil {
		return nil, err
	}
	vendorData, err := getMachineVendorData(series)
	if err != nil {
		return nil, err
	}
	for k, v := range vendorData {
		machineCloudInitData[k] = v
	}
	return machineCloudInitData, nil
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

// CloudInitCfgDir is for testing purposes.
var CloudInitCfgDir = paths.CloudInitCfgDir

// getMachineCloudCfgDirData returns a map of the combined machine's cloud init
// cloud.cfg.d config files.  Files are read in lexical order.
func getMachineCloudCfgDirData(series string) (map[string]interface{}, error) {
	dir, err := CloudInitCfgDir(series)
	if err != nil {
		return nil, errors.Annotate(err, "cannot determine CloudInitCfgDir for the machine")
	}
	fileInfo, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, errors.Annotate(err, "cannot determine files in CloudInitCfgDir for the machine")
	}
	sortedFileInfos := sortableFileInfos(fileInfo)
	sort.Sort(sortedFileInfos)
	cloudInit := make(map[string]interface{})
	for _, file := range fileInfo {
		name := file.Name()
		if !strings.HasSuffix(name, ".cfg") {
			continue
		}
		data, err := ioutil.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, errors.Annotatef(err, "cannot read %q from machine", name)
		}
		cloudCfgData, err := unmarshallContainerCloudInit(data)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot unmarshall %q from machine", name)
		}
		for k, v := range cloudCfgData {
			cloudInit[k] = v
		}
	}
	return cloudInit, nil
}

// getMachineVendorData returns a map of machine's cloud init vendor-data.txt.
func getMachineVendorData(series string) (map[string]interface{}, error) {
	// vendor-data.txt may or may not be compressed.
	return getMachineData(series, "vendor-data.txt")
}

// MachineCloudInitDir is for testing purposes.
var MachineCloudInitDir = paths.MachineCloudInitDir

func getMachineData(series, file string) (map[string]interface{}, error) {
	dir, err := MachineCloudInitDir(series)
	if err != nil {
		return nil, errors.Annotate(err, "cannot determine MachineCloudInitDir for the machine")
	}
	data, err := ioutil.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return nil, errors.Annotatef(err, "cannot read %q from machine", file)
	}

	if len(data) == 0 {
		// vendor-data.txt is sometimes empty
		return nil, nil
	}
	// The userdata maybe be gzip'd, base64 encoded, both, or neither.
	// If both, it's been gzip'd, then base64 encoded.
	rawBuf, err := unmarshallContainerCloudInit(data)
	if err == nil {
		return rawBuf, nil
	}
	logger.Tracef("unmarshal of %q failed (%s), maybe it is compressed", file, err)

	zippedData, err := utils.Gunzip(data)
	if err == nil {
		return unmarshallContainerCloudInit(zippedData)
	}
	logger.Tracef("Gunzip of %q failed (%s), maybe it is encoded", file, err)

	decodedData, err := base64.StdEncoding.DecodeString(string(data))
	if err == nil {
		// it could still be gzip'd.
		buf, err := unmarshallContainerCloudInit(decodedData)
		if err == nil {
			return buf, nil
		}
	}
	logger.Tracef("Decoding of %q failed (%s), maybe it is encoded and gzipped", file, err)

	decodedZippedBuf, err := utils.Gunzip(decodedData)
	if err != nil {
		// During testing, it was found that the trusty vendor-data.txt.i file
		// can contain only the text "NONE", which doesn't unmarshall or decompress
		// we don't want to fail in that case.
		if series == "trusty" {
			logger.Debugf("failed to unmarshall or decompress %q: %s", file, err)
			return nil, nil
		}
		return nil, errors.Annotatef(err, "cannot unmarshall or decompress %q", file)
	}
	return unmarshallContainerCloudInit(decodedZippedBuf)
}

func unmarshallContainerCloudInit(raw []byte) (map[string]interface{}, error) {
	dataMap := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(raw), &dataMap)
	if err != nil {
		return nil, err
	}
	return dataMap, nil
}

type cloudConfigTranslateFunc func(string, map[string]interface{}, loggo.Logger) map[string]interface{}

// CloudConfigByVersionFunc returns the correct function to translate
// container-inherit-properties to cloud-init data based on series.
func CloudConfigByVersionFunc(series string) cloudConfigTranslateFunc {
	if series == "trusty" {
		return machineCloudConfigV077
	}
	// There is a big assumption that supported CentOS and OpenSUSE versions
	// supported by juju are using cloud-init version >= 0.7.8
	return machineCloudConfigV078
}

// machineCloudConfigV078 finds the containerInheritProperties properties and
// values in the given dataMap and returns a cloud-init v0.7.8 formatted map.
func machineCloudConfigV078(containerInheritProperties string, dataMap map[string]interface{}, log loggo.Logger) map[string]interface{} {
	if containerInheritProperties == "" {
		return nil
	}
	foundDataMap := make(map[string]interface{})
	for _, k := range strings.Split(containerInheritProperties, ",") {
		key := strings.TrimSpace(k)
		switch key {
		case "apt-security", "apt-sources", "apt-primary":
			if val, ok := dataMap["apt"]; ok {
				for k, v := range machineCloudConfigAptV078(key, val, log) {
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
				log.Debugf("%s not found in machine cloud-init data", key)
			}
		case "ca-certs":
			// no translation needed, ca-certs the same in both versions of cloudinit
			if val, ok := dataMap[key]; ok {
				foundDataMap[key] = val
			} else {
				log.Debugf("%s not found in machine cloud-init data", key)
			}
		}
	}
	return foundDataMap
}

func machineCloudConfigAptV078(key string, val interface{}, log loggo.Logger) map[string]interface{} {
	split := strings.Split(key, "-")
	secondary := split[1]

	for k, v := range interfaceToMapStringInterface(val) {
		if k == secondary {
			foundDataMap := make(map[string]interface{})
			foundDataMap[k] = v
			return foundDataMap
		}
	}

	log.Debugf("%s not found in machine cloud-init data", key)
	return nil
}

var aptPrimaryKeys = []string{"apt_mirror", "apt_mirror_search", "apt_mirror_search_dns"}
var aptSourcesKeys = []string{"apt_sources"}

// machineCloudConfigV077 finds the containerInheritProperties properties and
// values in the given dataMap and returns a cloud-init v0.7.7 formatted map.
func machineCloudConfigV077(containerInheritProperties string, dataMap map[string]interface{}, log loggo.Logger) map[string]interface{} {
	if containerInheritProperties == "" {
		return nil
	}
	foundDataMap := make(map[string]interface{})
	keySplit := strings.Split(containerInheritProperties, ",")
	for _, k := range keySplit {
		key := strings.TrimSpace(k)
		switch key {
		case "apt-primary", "apt-sources":
			for _, aptKey := range append(aptPrimaryKeys, aptSourcesKeys...) {
				if val, ok := dataMap[aptKey]; ok {
					foundDataMap[aptKey] = val
				} else {
					log.Debugf("%s not found as part of %s, in machine cloud-init data", strings.Join(keySplit, "-"), key)
				}
			}
		case "apt-security":
			// Translation for apt-security unknown at this time.
			log.Debugf("%s not found in machine cloud-init data", key)
		case "ca-certs":
			// no translation needed, ca-certs the same in both versions of cloudinit
			if val, ok := dataMap[key]; ok {
				foundDataMap[key] = val
			} else {
				log.Debugf("%s not found in machine cloud-init data", key)
			}
		}
	}
	return foundDataMap
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
