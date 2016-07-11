import logging
import yaml
import json

# Tested on Azure 2.0 API
from azure.common.credentials import ServicePrincipalCredentials
from azure.mgmt.compute import (
    ComputeManagementClient,
    )
import azure.mgmt.resource.subscriptions
from msrestazure.azure_exceptions import CloudError


def get_credentials(azure_dict):
    return ServicePrincipalCredentials(
        client_id=azure_dict['application-id'],
        secret=azure_dict['application-password'],
        tenant=azure_dict['tenant-id'],
        subscription_id=azure_dict['subscription-id'],
        )


def get_client(azure_dict, credentials, sub):
    return ComputeManagementClient(get_credentials(azure_dict), azure_dict['subscription-id'])


def get_publishers(client, region):
    vm_images = client.virtual_machine_images
    vm_images.list_publishers('westus')
    return dict((p.name, p) for p in vm_images.list_publishers('westus'))


def get_images(client, region, region_name):
    #publishers = get_publishers(client, region)
    MS_VSTUDIO = 'MicrosoftVisualStudio'
    MS_SERVER = 'MicrosoftWindowsServer'
    WINDOWS = 'Windows'
    WINDOWS_SERVER = 'WindowsServer'
    image_spec = {
        'win81': (MS_VSTUDIO, WINDOWS, '8.1-Enterprise-N'),
#        'win10': (MS_VSTUDIO, WINDOWS, '10-Enterprise-N'),
#        'win2012': (MS_SERVER, WINDOWS_SERVER, '2012-Datacenter'),
#        'win2012r2': (MS_SERVER, WINDOWS_SERVER, '2012-R2-Datacenter'),
#        'centos7': ('OpenLogic', 'CentOS', '7.1'),
#        'asdf': ('Canonical', 'UbuntuServer', '16.04.0-LTS'),
    }
    stanzas = []
    for name, spec in image_spec.items():
        try:
            versions = client.virtual_machine_images.list(region, *spec)
        except CloudError:
            logging.warning('Could not find {} {} {} in {region}'.format(*spec,
                            region=region))
            continue
        for version in versions:
            stanzas.append({
                'content_id': 'com.ubuntu.cloud:released:azure',
                'region': region_name,
                'virt': 'Hyper-V',
                'version': version.name,
                'id': version.id,
            })
    return stanzas



def main():
    with open('/home/abentley/canonical/cloud-city/credentials.yaml') as f:
        cred_dict = yaml.safe_load(f)
    azure_dict = cred_dict['credentials']['azure']['credentials']
    subscription_id = azure_dict['subscription-id']
    credentials = get_credentials(azure_dict)
    sub_client = azure.mgmt.resource.subscriptions.SubscriptionClient(
        credentials)
    client = ComputeManagementClient(credentials, subscription_id)
    images = []
    for region in sub_client.subscriptions.list_locations(subscription_id):
        images.extend(get_images(client, region.name, region.display_name))
    with open('stanzas.json', 'w') as stanza_file:
        json.dump(images, stanza_file, indent=2, sort_keys=True)


if __name__ == '__main__':
    main()
