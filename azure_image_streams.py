import logging
import yaml

# Tested on Azure 2.0 API
from azure.common.credentials import ServicePrincipalCredentials
from azure.mgmt.compute import (
    ComputeManagementClient,
    )
from azure.mgmt.resource.subscriptions import SubscriptionClient
from msrestazure.azure_exceptions import CloudError
from simplestreams.json2streams import Item

from make_aws_image_streams import (
    get_parameters,
    make_aws_items,
    write_item_streams,
    )


MS_VSTUDIO = 'MicrosoftVisualStudio'
MS_SERVER = 'MicrosoftWindowsServer'
WINDOWS = 'Windows'
WINDOWS_SERVER = 'WindowsServer'
IMAGE_SPEC = [
    ('win81', MS_VSTUDIO, WINDOWS, '8.1-Enterprise-N'),
    ('win10', MS_VSTUDIO, WINDOWS, '10-Enterprise-N'),
    ('win2012', MS_SERVER, WINDOWS_SERVER, '2012-Datacenter'),
    ('win2012r2', MS_SERVER, WINDOWS_SERVER, '2012-R2-Datacenter'),
    ('centos7', 'OpenLogic', 'CentOS', '7.1'),
]


def get_azure_credentials(all_credentials):
    azure_dict = all_credentials['azure']['credentials']
    subscription_id = azure_dict['subscription-id']
    return subscription_id, ServicePrincipalCredentials(
        client_id=azure_dict['application-id'],
        secret=azure_dict['application-password'],
        tenant=azure_dict['tenant-id'],
        subscription_id=azure_dict['subscription-id'],
        )


def get_image_versions(client, region, region_name):
    endpoint = client.config.base_url
    for full_spec in IMAGE_SPEC:
        spec = full_spec[1:]
        try:
            versions = client.virtual_machine_images.list(region, *spec)
        except CloudError:
            logging.warning('Could not find {} {} {} in {region}'.format(*spec,
                            region=region))
            continue
        for version in versions:
            yield make_item(version, full_spec, region_name, endpoint)


def make_item(version, full_spec, region_name, endpoint):
    URN = ':'.join(full_spec[1:] + (version.name,))
    product_name = (
        'com.ubuntu.cloud:server:centos7:amd64' if full_spec[2] == 'CentOS'
        else 'com.ubuntu.cloud:windows')
    return Item(
        'com.ubuntu.cloud:released:azure',
        product_name,
        version.name, version.location, {
            'arch': 'amd64',
            'virt': 'Hyper-V',
            'region': region_name,
            'id': URN,
            'label': 'release',
            'endpoint': endpoint,
            'release': full_spec[0],
            }
        )


def make_azure_items(all_credentials):
    subscription_id, credentials = get_azure_credentials(all_credentials)
    sub_client = SubscriptionClient(credentials)
    client = ComputeManagementClient(credentials, subscription_id)
    items = []
    for region in sub_client.subscriptions.list_locations(subscription_id):
        logging.info('Retrieving image data in {}'.format(region.display_name))
        items.extend(get_image_versions(
            client, region.name, region.display_name))
    return items
