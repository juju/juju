// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

// Generated code - do not edit.

const fallbackPublicCloudInfo = `
# DO NOT EDIT, will be overwritten, use “juju update-clouds” to refresh.
clouds:
  aws:
    type: ec2
    auth-types: [ access-key ]
    regions:
      us-east-1:
        endpoint: https://us-east-1.aws.amazon.com/v1.2/
      us-west-1:
        endpoint: https://us-west-1.aws.amazon.com/v1.2/
      us-west-2:
        endpoint: https://us-west-2.aws.amazon.com/v1.2/
      eu-west-1:
        endpoint: https://eu-west-1.aws.amazon.com/v1.2/
      eu-central-1:
        endpoint: https://eu-central-1.aws.amazon.com/v1.2/
      ap-southeast-1:
        endpoint: https://ap-southeast-1.aws.amazon.com/v1.2/
      ap-southeast-2:
        endpoint: https://ap-southeast-2.aws.amazon.com/v1.2/
      ap-northeast-1:
        endpoint: https://ap-northeast-1.aws.amazon.com/v1.2/
      sa-east-1:
        endpoint: https://sa-east-1.aws.amazon.com/v1.2/
  aws-china:
    type: ec2
    auth-types: [ access-key ]
    regions:
      cn-north-1:
        endpoint: https://ec2.cn-north-1.amazonaws.com.cn/
  aws-gov:
    type: ec2
    auth-types: [ access-key ]
    regions:
      us-gov-west-1:
        endpoint: https://ec2.us-gov-west-1.amazonaws-govcloud.com
  google:
    type: gce
    auth-types: [ oauth2 ]
    regions:
      us-east1:
        endpoint: https://www.googleapis.com
      us-central1:
        endpoint: https://www.googleapis.com
      europe-west1:
        endpoint: https://www.googleapis.com
      asia-east1:
        endpoint: https://www.googleapis.com
  azure:
    type: azure
    auth-types: [ userpass ]
    regions:
      Central US:
        endpoint: https://management.azure.com
      East US:
        endpoint: https://management.azure.com
      East US 2:
        endpoint: https://management.azure.com
      North Central US:
        endpoint: https://management.azure.com
      South Central US:
        endpoint: https://management.azure.com
      West US:
        endpoint: https://management.azure.com
      North Europe:
        endpoint: https://management.azure.com
      West Europe:
        endpoint: https://management.azure.com
      East Asia:
        endpoint: https://management.azure.com
      Southeast Asia:
        endpoint: https://management.azure.com
      Japan East:
        endpoint: https://management.azure.com
      Japan West:
        endpoint: https://management.azure.com
      Brazil South:
        endpoint: https://management.azure.com
      Australia East:
        endpoint: https://management.azure.com
      Australia Southeast:
        endpoint: https://management.azure.com
      Central India:
        endpoint: https://management.azure.com
      South India:
        endpoint: https://management.azure.com
      West India:
        endpoint: https://management.azure.com
  azure-china:
    type: azure
    auth-types: [ userpass ]
    regions:
      China East:
        endpoint: https://management.chinacloudapi.cn
      China North:
        endpoint: https://management.chinacloudapi.cn
  rackspace:
    type: openstack
    auth-types: [ access-key, userpass ]
    endpoint: https://identity.api.rackspacecloud.com/v2.0
    regions:
      Dallas-Fort Worth:
        endpoint: https://identity.api.rackspacecloud.com/v2.0
      Chicago:
        endpoint: https://identity.api.rackspacecloud.com/v2.0
      Northern Virginia:
        endpoint: https://identity.api.rackspacecloud.com/v2.0
      London:
        endpoint: https://lon.identity.api.rackspacecloud.com/v2.0
      Sydney:
        endpoint: https://identity.api.rackspacecloud.com/v2.0
      Hong Kong:
        endpoint: https://identity.api.rackspacecloud.com/v2.0
  joyent:
    type: joyent
    auth-types: [ userpass ]
    regions:
      eu-ams-1: 
        endpoint: https://eu-ams-1.api.joyentcloud.com
      us-sw-1: 
        endpoint: https://us-sw-1.api.joyentcloud.com
      us-east-1: 
        endpoint: https://us-east-.api.joyentcloud.com
      us-east-2: 
        endpoint: https://us-east--2.api.joyentcloud.com
      us-east-3: 
        endpoint: https://us-east-3.api.joyentcloud.com
      us-west-1: 
        endpoint: https://us-west-1.api.joyentcloud.com
  cloudsigma:
    type: cloudsigma
    auth-types: [ userpass ]
    regions:
      hnl:
        endpoint: https://hnl.cloudsigma.com/api/2.0/
      mia:
        endpoint: https://mia.cloudsigma.com/api/2.0/
      sjc:
        endpoint: https://sjc.cloudsigma.com/api/2.0/
      wdc:
        endpoint: https://wdc.cloudsigma.com/api/2.0/
      zrh:
        endpoint: https://zrh.cloudsigma.com/api/2.0/
`
