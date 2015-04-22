// Copyright 2011, 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

var _ CloudConfig = (*ubuntuCloudConfig)(nil)
var _ CloudConfig = (*centOSCloudConfig)(nil)
var _ CloudConfig = (*windowsCloudConfig)(nil)
