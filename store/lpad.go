package store

import (
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/log"
	"launchpad.net/lpad"
	"strings"
	"time"
)

type PublishBranchError struct {
	URL string
	Err error
}

type PublishBranchErrors []PublishBranchError

func (errs PublishBranchErrors) Error() string {
	return fmt.Sprintf("%d branch(es) failed to be published", len(errs))
}

// PublishCharmsDistro publishes all branch tips found in
// the /charms distribution in Launchpad onto store under
// the "cs:" scheme.
// apiBase specifies the Launchpad base API URL, such
// as lpad.Production or lpad.Staging.
// Errors found while processing one or more branches are
// all returned as a PublishBranchErrors value.
func PublishCharmsDistro(store *Store, apiBase lpad.APIBase) error {
	oauth := &lpad.OAuth{Anonymous: true, Consumer: "juju"}
	root, err := lpad.Login(apiBase, oauth)
	if err != nil {
		return err
	}
	distro, err := root.Distro("charms")
	if err != nil {
		return err
	}
	tips, err := distro.BranchTips(time.Time{})
	if err != nil {
		return err
	}

	var errs PublishBranchErrors
	for _, tip := range tips {
		if !strings.HasSuffix(tip.UniqueName, "/trunk") {
			continue
		}
		burl, curl, err := uniqueNameURLs(tip.UniqueName)
		if err != nil {
			errs = append(errs, PublishBranchError{tip.UniqueName, err})
			log.Errf("%v\n", err)
			continue
		}
		log.Infof("----- %s\n", burl)
		if tip.Revision == "" {
			errs = append(errs, PublishBranchError{burl, fmt.Errorf("branch has no revisions")})
			log.Errf("branch has no revisions\n")
			continue
		}
		// Charm is published in the personal URL and in any explicitly
		// assigned official series.
		urls := []*charm.URL{curl}
		schema, name := curl.Schema, curl.Name
		for _, series := range tip.OfficialSeries {
			curl = &charm.URL{Schema: schema, Name: name, Series: series, Revision: -1}
			curl.Series = series
			curl.User = ""
			urls = append(urls, curl)
		}

		err = PublishBazaarBranch(store, urls, burl, tip.Revision)
		if err == ErrRedundantUpdate {
			continue
		}
		if err != nil {
			errs = append(errs, PublishBranchError{burl, err})
			log.Errf("%v\n", err)
		}
	}
	if errs != nil {
		return errs
	}
	return nil
}

// uniqueNameURLs returns the branch URL and the charm URL for the
// provided Launchpad branch unique name. The unique name must be
// in the form:
//
//     ~<user>/charms/<series>/<charm name>/trunk
//
// For testing purposes, if name has a prefix preceding a string in
// this format, the prefix is stripped out for computing the charm
// URL, and the unique name is returned unchanged as the branch URL.
func uniqueNameURLs(name string) (burl string, curl *charm.URL, err error) {
	u := strings.Split(name, "/")
	if len(u) > 5 {
		u = u[len(u)-5:]
		burl = name
	} else {
		burl = "lp:" + name
	}
	if len(u) < 5 || u[1] != "charms" || u[4] != "trunk" || len(u[0]) == 0 || u[0][0] != '~' {
		return "", nil, fmt.Errorf("unwanted branch name: %s", name)
	}
	curl, err = charm.ParseURL(fmt.Sprintf("cs:%s/%s/%s", u[0], u[2], u[3]))
	if err != nil {
		return "", nil, err
	}
	return burl, curl, nil
}
