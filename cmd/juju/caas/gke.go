// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/juju/interact"
	k8s "github.com/juju/juju/internal/provider/caas/kubernetes"
	"github.com/juju/juju/internal/provider/caas/kubernetes/clientconfig"
)

type gke struct {
	CommandRunner
}

func newGKECluster() k8sCluster {
	return &gke{CommandRunner: &defaultRunner{}}
}

func (g *gke) cloud() string {
	return k8s.K8sCloudGCE
}

func (g *gke) ensureExecutable() error {
	whichCmd := []string{"which", "gcloud"}
	err := collapseRunError(runCommand(g, whichCmd, ""))
	errAnnotationMessage := "gcloud command not found, please 'snap install google-cloud-sdk --classic' then try again"
	if err != nil {
		return errors.Annotate(err, errAnnotationMessage)
	}
	return nil
}

func (g *gke) getKubeConfig(p *clusterParams) (io.ReadCloser, string, error) {
	gcloudCmd := []string{
		"gcloud", "container", "clusters", "get-credentials", p.name,
	}
	qualifiedClusterName := "gke_"
	if p.credential != "" {
		gcloudCmd = append(gcloudCmd, "--account", p.credential)
	}
	if p.project != "" {
		gcloudCmd = append(gcloudCmd, "--project", p.project)
		qualifiedClusterName += p.project + "_"
	}
	if p.zone != "" {
		gcloudCmd = append(gcloudCmd, "--zone", p.zone)
		qualifiedClusterName += p.zone + "_"
	}
	qualifiedClusterName += p.name

	kubeconfig := clientconfig.GetKubeConfigPath()
	result, err := runCommand(g, gcloudCmd, kubeconfig)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	if result.Code != 0 {
		return nil, "", errors.New(string(result.Stderr))
	}
	rdr, err := p.openFile(kubeconfig)
	return rdr, qualifiedClusterName, err
}

func (g *gke) interactiveParams(ctxt *cmd.Context, p *clusterParams) (*clusterParams, error) {
	errout := interact.NewErrWriter(ctxt.Stdout)
	pollster := interact.New(ctxt.Stdin, ctxt.Stdout, errout)

	var err error
	if p.credential == "" {
		p.credential, err = g.queryAccount(pollster)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	if p.project == "" {
		p.project, err = g.queryProject(pollster, p.credential)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	if p.name == "" {
		p.name, p.region, p.zone, err = g.queryCluster(pollster, p.credential, p.project, p.region)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return p, nil
}

func (g *gke) listAccounts() ([]string, string, error) {
	gcloudCmd := []string{
		"gcloud", "auth", "list", "--format", "value\\(account,status\\)",
	}
	result, err := runCommand(g, gcloudCmd, "")
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	if result.Code != 0 {
		return nil, "", errors.New(string(result.Stderr))
	}
	info := strings.Split(string(result.Stdout), "\n")

	var accounts []string
	var defaultAccount string
	for _, line := range info {
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		accounts = append(accounts, parts[0])
		if len(parts) > 1 && parts[1] == "*" {
			defaultAccount = parts[0]
		}
	}
	return accounts, defaultAccount, nil
}

func (g *gke) queryAccount(pollster *interact.Pollster) (string, error) {
	allAccounts, defaultAccount, err := g.listAccounts()
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(allAccounts) == 0 {
		return "", errors.New("no accounts have been set up.\n" +
			"See 'gcloud help auth'.",
		)
	}
	if defaultAccount == "" {
		defaultAccount = allAccounts[0]
	}
	account, err := pollster.Select(interact.List{
		Singular: "account",
		Plural:   "Available accounts",
		Options:  allAccounts,
		Default:  defaultAccount,
	})
	return account, errors.Trace(err)
}

func (g *gke) listProjects(account string) ([]string, error) {
	gcloudCmd := []string{
		"gcloud", "projects", "list", "--account", account, "--filter", "lifecycleState:ACTIVE", "--format", "value\\(projectId\\)",
	}
	result, err := runCommand(g, gcloudCmd, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Code != 0 {
		return nil, errors.New(string(result.Stderr))
	}
	return strings.Split(string(result.Stdout), "\n"), nil
}

func (g *gke) queryProject(pollster *interact.Pollster, account string) (string, error) {
	allProjects, err := g.listProjects(account)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(allProjects) == 0 {
		return "", errors.New("no projects have been set up.\n" +
			"You can create a project using 'gcloud projects create'",
		)
	}
	project, err := pollster.Select(interact.List{
		Singular: "project",
		Plural:   "Available projects",
		Options:  allProjects,
		Default:  allProjects[0],
	})
	return project, errors.Trace(err)
}

var extractRegionFromZone = regexp.MustCompile(`([a-z]+-[a-z0-9]+)`).FindStringSubmatch

func (g *gke) listClusters(account, project, region string) (map[string]cluster, error) {
	gcloudCmd := []string{
		"gcloud", "container", "clusters", "list", "--filter", "status:RUNNING", "--account", account, "--project", project, "--format", "value\\(name,zone\\)",
	}
	result, err := runCommand(g, gcloudCmd, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Code != 0 {
		return nil, errors.New(string(result.Stderr))
	}
	info := strings.Split(string(result.Stdout), "\n")

	clusters := make(map[string]cluster)
	for _, line := range info {
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		c := cluster{name: parts[0], region: region}
		if len(parts) > 1 {
			c.zone = parts[1]
			result := extractRegionFromZone(c.zone)
			if result != nil && len(result) > 0 {
				r := result[0]
				if region != "" && region != r {
					continue
				}
				c.region = r
			}
		}
		clusters[c.name] = c
	}
	return clusters, nil
}

func (g *gke) queryCluster(pollster *interact.Pollster, account, project, region string) (string, string, string, error) {
	allClustersByName, err := g.listClusters(account, project, region)
	if err != nil {
		return "", "", "", errors.Trace(err)
	}
	if len(allClustersByName) == 0 {
		regionMsg := ""
		if region != "" {
			regionMsg = fmt.Sprintf(" in region %v", region)
		}
		return "", "", "", errors.Errorf("no clusters have been set up%s.\n"+
			"You can create a k8s cluster using 'gcloud container cluster create'",
			regionMsg,
		)
	}
	var clusterNamesAndRegions []string
	clustersByNameRegion := make(map[string]cluster)
	for n, c := range allClustersByName {
		nr := n
		if c.region != "" {
			nr += " in " + c.region
		}
		clusterNamesAndRegions = append(clusterNamesAndRegions, nr)
		clustersByNameRegion[nr] = c
	}
	cluster, err := pollster.Select(interact.List{
		Singular: "cluster",
		Plural:   "Available clusters",
		Options:  clusterNamesAndRegions,
		Default:  clusterNamesAndRegions[0],
	})
	if err != nil {
		return "", "", "", errors.Trace(err)
	}
	selected := clustersByNameRegion[cluster]
	return selected.name, selected.region, selected.zone, nil
}
