// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/interact"
)

// assembleClouds
func assembleClouds() ([]string, error) {
	public, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}

	personal, err := jujucloud.PersonalCloudMetadata()
	if err != nil {
		return nil, errors.Trace(err)
	}

	builtin, err := common.BuiltInClouds()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return sortClouds(public, builtin, personal), nil
}

// queryCloud asks the user to choose a cloud.
func queryCloud(clouds []string, defCloud string, scanner *bufio.Scanner, w io.Writer) (string, error) {
	list := strings.Join(clouds, "\n")
	if _, err := fmt.Fprint(w, "Clouds\n", list, "\n\n"); err != nil {
		return "", errors.Trace(err)
	}

	// add support for a default (empty) selection.
	clouds = append(clouds, "")

	verify := interact.MatchOptions(clouds, "Invalid cloud.")

	query := fmt.Sprintf("Select a cloud [%s]: ", defCloud)
	cloud, err := interact.QueryVerify(query, scanner, w, w, verify)
	if err != nil {
		return "", errors.Trace(err)
	}
	if cloud == "" {
		return defCloud, nil
	}
	if ok := names.IsValidCloud(cloud); !ok {
		return "", errors.NotValidf("cloud name %q", cloud)
	}

	cloudName, ok := interact.FindMatch(cloud, clouds)
	if !ok {
		// should be impossible
		return "", errors.Errorf("invalid cloud name chosen: %s", cloud)
	}

	return cloudName, nil
}

// queryRegion asks the user to pick a region of the ones passed in. The first
// region in the list will be the default.
func queryRegion(cloud string, regions []jujucloud.Region, scanner *bufio.Scanner, w io.Writer) (string, error) {
	fmt.Fprintf(w, "Regions in %s:\n", cloud)
	names := jujucloud.RegionNames(regions)
	// add an empty string to allow for a default value. Also gives us an extra
	// line return after the list of names.
	names = append(names, "")
	if _, err := fmt.Fprintln(w, strings.Join(names, "\n")); err != nil {
		return "", errors.Trace(err)
	}
	verify := interact.MatchOptions(names, "Invalid region.")
	defaultRegion := regions[0].Name
	query := fmt.Sprintf("Select a region in %s [%s]: ", cloud, defaultRegion)
	region, err := interact.QueryVerify(query, scanner, w, w, verify)
	if err != nil {
		return "", errors.Trace(err)
	}
	if region == "" {
		return defaultRegion, nil
	}
	regionName, ok := interact.FindMatch(region, names)
	if !ok {
		// should be impossible
		return "", errors.Errorf("invalid region name chosen: %s", region)
	}

	return regionName, nil
}

func defaultControllerName(cloudname, region string) string {
	if region == "" {
		return cloudname
	}
	return cloudname + "-" + region
}

func queryName(defName string, scanner *bufio.Scanner, w io.Writer) (string, error) {
	query := fmt.Sprintf("Enter a name for the Controller [%s]: ", defName)
	name, err := interact.QueryVerify(query, scanner, w, w, nil)
	if err != nil {
		return "", errors.Trace(err)
	}
	if name == "" {
		return defName, nil
	}
	return name, nil
}

func sortClouds(maps ...map[string]jujucloud.Cloud) []string {
	var clouds []string
	for _, m := range maps {
		for name := range m {
			clouds = append(clouds, name)
		}
	}
	sort.Strings(clouds)
	return clouds
}
