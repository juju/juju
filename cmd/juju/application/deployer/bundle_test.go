// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

//func (s *BundleDeployCharmStoreSuite) TestDeployBundleLocalDeploymentBadConfig(c *gc.C) {
//	charmsPath := c.MkDir()
//	mysqlPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "mysql")
//	wordpressPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "wordpress")
//	err := s.DeployBundleYAML(c, fmt.Sprintf(`
//        series: xenial
//        applications:
//            wordpress:
//                charm: %s
//                num_units: 1
//            mysql:
//                charm: %s
//                num_units: 2
//        relations:
//            - ["wordpress:db", "mysql:server"]
//    `, wordpressPath, mysqlPath),
//		"--overlay", "missing-file")
//	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: unable to process overlays: "missing-file" not found`)
//}
