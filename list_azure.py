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
from simplestreams.generate_simplestreams import items2content_trees
from simplestreams.json2streams import Item
from simplestreams import util

from make_aws_image_streams import write_juju_streams


def get_credentials(azure_dict):
    return ServicePrincipalCredentials(
        client_id=azure_dict['application-id'],
        secret=azure_dict['application-password'],
        tenant=azure_dict['tenant-id'],
        subscription_id=azure_dict['subscription-id'],
        )


def get_image_versions(client, region):
    MS_VSTUDIO = 'MicrosoftVisualStudio'
    MS_SERVER = 'MicrosoftWindowsServer'
    WINDOWS = 'Windows'
    WINDOWS_SERVER = 'WindowsServer'
    image_spec = {
        'win81': (MS_VSTUDIO, WINDOWS, '8.1-Enterprise-N'),
        'win10': (MS_VSTUDIO, WINDOWS, '10-Enterprise-N'),
        'win2012': (MS_SERVER, WINDOWS_SERVER, '2012-Datacenter'),
        'win2012r2': (MS_SERVER, WINDOWS_SERVER, '2012-R2-Datacenter'),
        'centos7': ('OpenLogic', 'CentOS', '7.1'),
#        'asdf': ('Canonical', 'UbuntuServer', '16.04.0-LTS'),
    }
    stanzas = []
    items = []
    for name, spec in image_spec.items():
        try:
            versions = client.virtual_machine_images.list(region, *spec)
        except CloudError:
            logging.warning('Could not find {} {} {} in {region}'.format(*spec,
                            region=region))
            continue
        for version in versions:
            yield version


def make_item(version, region_name, endpoint):
    return Item(
        'com.ubuntu.cloud:released:azure',
        'com.ubuntu.cloud:windows',
        version.name, version.location, {
            'virt': 'Hyper-V',
            'region': region_name,
            'id': version.id,
            'label': 'release',
            'endpoint': endpoint
            }
        )


def write_streams(credentials, subscription_id, out_dir):
    sub_client = azure.mgmt.resource.subscriptions.SubscriptionClient(
        credentials)
    client = ComputeManagementClient(credentials, subscription_id)
    items = []
    for region in sub_client.subscriptions.list_locations(subscription_id):
        for version in get_image_versions(client, region.name):
            items.append(make_item(version, region.display_name,
                                   client.config.base_url))
    updated = util.timestamp()
    data = {'updated': updated, 'datatype': 'image-ids'}
    trees = items2content_trees(items, data)
    write_juju_streams(out_dir, trees, updated, [
        'path', 'sha256', 'md5', 'size', 'virt', 'root_store'])



def main():
    with open('/home/abentley/canonical/cloud-city/credentials.yaml') as f:
        cred_dict = yaml.safe_load(f)
    azure_dict = cred_dict['credentials']['azure']['credentials']
    subscription_id = azure_dict['subscription-id']
    credentials = get_credentials(azure_dict)
    write_streams(credentials, subscription_id, 'outdir')


if __name__ == '__main__':
    main()
