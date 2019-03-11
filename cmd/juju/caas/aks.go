// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cmd/juju/interact"
)

type aks struct {
	CommandRunner
	azExecName string
}

func newAKSCluster() k8sCluster {
	return &aks{CommandRunner: &defaultRunner{}}
}

func (a *aks) cloud() string {
	return "aks"
}

func (a *aks) ensureExecutable() error {
	if a.azExecName != "" {
		return nil
	}

	cmd := []string{"which", "az"}
	err := collapseRunError(runCommand(a, cmd, ""))
	if err != nil {
		cmd = []string{"which", "azure-cli"}
		err := collapseRunError(runCommand(a, cmd, ""))
		errAnnotationMessage := "neither az or azure-cli (version > 2.0.52) found. Please install, login, and try again"
		if err != nil {
			return errors.Annotate(err, errAnnotationMessage)
		}
		a.azExecName = "azure-cli"
	} else {
		a.azExecName = "az"
	}

	// check that we are logged in, there is no way to provide login details to a separate command.
	cmd = []string{a.azExecName, "account", "show"}
	err = collapseRunError(runCommand(a, cmd, ""))
	errAnnotationMessage := fmt.Sprintf("please run '%s login' to setup account", a.azExecName)
	if err != nil {
		return errors.Annotate(err, errAnnotationMessage)
	}
	return nil
}

func (a *aks) getKubeConfig(p *clusterParams) (io.ReadCloser, string, error) {
	// 'az aks get-credential' ignores KUBECONFIG env var and instead relies on -f.
	kubeconfig := clientconfig.GetKubeConfigPath()
	cmd := []string{
		a.azExecName, "aks", "get-credentials",
		"--name", p.name, "--resource-group", p.resourceGroup,
		"--overwrite-existing",
		"-f", kubeconfig,
	}

	err := collapseRunError(runCommand(a, cmd, kubeconfig))
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	reader, err := os.Open(kubeconfig)
	return reader, p.name, err
}

func (a *aks) interactiveParams(ctxt *cmd.Context, p *clusterParams) (*clusterParams, error) {
	errout := interact.NewErrWriter(ctxt.Stdout)
	pollster := interact.New(ctxt.Stdin, ctxt.Stdout, errout)

	var err error
	if p.name == "" {
		p.name, p.resourceGroup, err = a.queryCluster(pollster, p.resourceGroup)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	if p.resourceGroup == "" {
		clusters, err := a.listNamedClusters(p.name, p.resourceGroup)
		if err != nil {
			return nil, errors.Trace(err)
		}

		if len(clusters) == 0 {
			resourceGroupMsg := ""
			if p.resourceGroup != "" {
				resourceGroupMsg = fmt.Sprintf(" in resource group %s", p.resourceGroup)
			}
			return nil, errors.Errorf(
				"cluster %q not found%s.\nSee '%s aks create --help'", p.name, resourceGroupMsg, a.azExecName)
		}

		if len(clusters) == 1 {
			p.resourceGroup = clusters[0].resourceGroup
		} else {
			// we have multiple clusters with the same name but different resource groups
			p.resourceGroup, err = a.queryResourceGroupsForClusters(pollster, clusters)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
	}

	return p, nil
}

func (a *aks) queryResourceGroupsForClusters(pollster *interact.Pollster, clusters []cluster) (string, error) {
	groups, err := a.listResourceGroups(clusters)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(groups) == 0 {
		return "", errors.New("no resource groups found.\n" +
			fmt.Sprintf("see '%s group --help'", a.azExecName))
	}

	var displayResourceGroupOptions []string
	resourceGroupLookup := make(map[string]string)
	for _, rg := range groups {
		displayName := fmt.Sprintf("%s in %s", rg.Name, rg.Location)
		displayResourceGroupOptions = append(displayResourceGroupOptions, displayName)
		resourceGroupLookup[displayName] = rg.Name
	}
	groupDisplayName, err := pollster.Select(interact.List{
		Singular: "resource group",
		Plural:   "Available resource groups",
		Options:  displayResourceGroupOptions,
		Default:  displayResourceGroupOptions[0],
	})
	group := resourceGroupLookup[groupDisplayName]
	return group, errors.Trace(err)
}

type resourceGroupDetails struct {
	Name     string `json:"name"`
	Location string `json:"location"`
}

// will list resource groups used by the passed clusters
func (a *aks) listResourceGroups(clusters []cluster) ([]resourceGroupDetails, error) {
	usedRG := set.Strings{}
	for _, c := range clusters {
		usedRG.Add(c.resourceGroup)
	}

	// It seems that any resource group that has a non-null 'managedBy' is a
	// generated RG i.e. via the creation of the cluster itself.
	cmd := []string{
		a.azExecName, "group", "list",
		"--output", "json",
		"--query", `"[?properties.provisioningState=='Succeeded']"`,
	}
	result, err := runCommand(a, cmd, "")
	err = collapseRunError(result, err)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var groups []resourceGroupDetails
	if err := json.Unmarshal(result.Stdout, &groups); err != nil {
		return nil, errors.Trace(err)
	}

	var filteredGroups []resourceGroupDetails
	for _, group := range groups {
		if usedRG.Contains(group.Name) {
			filteredGroups = append(filteredGroups, group)
		}
	}

	return filteredGroups, nil
}

func (a *aks) queryCluster(pollster *interact.Pollster, resourceGroup string) (string, string, error) {
	allClusters, err := a.listClusters(resourceGroup)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	if len(allClusters) == 0 {
		resourceGroupMsg := ""
		if resourceGroup != "" {
			resourceGroupMsg = fmt.Sprintf(" in resource group %s", resourceGroup)
		}
		return "", "", errors.Errorf(
			"no clusters have been setup%s.\nSee '%s aks create --help'", resourceGroupMsg, a.azExecName)
	}

	var clusterNamer func(clusterName, resourceGroupName string) string
	var clusterNamesAndResourceGroups []string
	clusterRGLookup := make(map[string]cluster)
	clusterPluralText := "Available clusters"

	// Display the resource group name if there is more than one in the list
	// of clusters, otherwise just the cluster name is enough
	clusterNamer = func(clusterName, resourceGroupName string) string {
		return clusterName
	}
	if resourceGroup == "" {
		groupsMentioned := set.Strings{}
		for _, cluster := range allClusters {
			groupsMentioned.Add(cluster.resourceGroup)
		}
		if groupsMentioned.Size() > 1 {
			clusterNamer = func(clusterName, resourceGroupName string) string {
				return fmt.Sprintf("%s in resource group %s", clusterName, resourceGroupName)
			}
		} else {
			clusterPluralText = fmt.Sprintf("Available clusters in resource group %s", groupsMentioned.Values()[0])
		}
	}

	for _, cluster := range allClusters {
		namedGroup := clusterNamer(cluster.name, cluster.resourceGroup)
		clusterNamesAndResourceGroups = append(clusterNamesAndResourceGroups, namedGroup)
		clusterRGLookup[namedGroup] = cluster
	}
	cluster, err := pollster.Select(interact.List{
		Singular: "cluster",
		Plural:   clusterPluralText,
		Options:  clusterNamesAndResourceGroups,
		Default:  clusterNamesAndResourceGroups[0],
	})
	if err != nil {
		return "", "", errors.Trace(err)
	}
	selected := clusterRGLookup[cluster]
	return selected.name, selected.resourceGroup, nil
}

type clusterDetails struct {
	Name          string `json:"name"`
	ResourceGroup string `json:"resourceGroup"`
}

func (a *aks) listClusters(resourceGroup string) ([]cluster, error) {
	cmd := []string{
		a.azExecName, "aks", "list",
		"--output", "json",
	}
	if resourceGroup != "" {
		cmd = append(cmd, "--resource-group", resourceGroup)
	}
	result, err := runCommand(a, cmd, "")
	err = collapseRunError(result, err)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var clusterInfo []clusterDetails
	if err := json.Unmarshal(result.Stdout, &clusterInfo); err != nil {
		return nil, errors.Trace(err)
	}
	var clusters []cluster
	for _, ci := range clusterInfo {
		clusters = append(clusters, cluster{
			name:          ci.Name,
			resourceGroup: ci.ResourceGroup,
		})
	}
	return clusters, nil
}

func (a *aks) listNamedClusters(clusterName, resourceGroup string) ([]cluster, error) {
	allClusters, err := a.listClusters(resourceGroup)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var namedClusters []cluster
	for _, c := range allClusters {
		if c.name == clusterName {
			namedClusters = append(namedClusters, c)
		}
	}
	return namedClusters, nil
}
