package cloudconfig

import (
	"encoding/base64"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
	utilsos "github.com/juju/utils/os"
	utilsseries "github.com/juju/utils/series"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/juju/paths"
)

// GetMachineCloudInitData returns a map of all cloud init data on the machine.
func GetMachineCloudInitData(series string) (map[string]interface{}, error) {
	operatingSystem, err := utilsseries.GetOSFromSeries(series)
	if err != nil {
		return nil, err
	}

	switch operatingSystem {
	case utilsos.Ubuntu:
	case utilsos.CentOS:
	case utilsos.OpenSUSE:
	default:
		logger.Debugf("not attempting to get cloudinit data on %s", operatingSystem)
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
		return nil, errors.Annotatef(err, "cannot read %q from machine (%s)", file)
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
