import logging

# Tested on Azure 2.0 API
from azure.common.credentials import ServicePrincipalCredentials
from azure.mgmt.compute import (
    ComputeManagementClient,
    )
from azure.mgmt.resource.subscriptions import SubscriptionClient
from msrestazure.azure_exceptions import CloudError
from simplestreams.json2streams import Item


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


ITEM_NAMES = {
    "Australia East": "auee1i3",
    "Australia Southeast": "ause1i3",
    "Brazil South": "brss1i3",
    "Canada Central": "cacc1i3",
    "Canada East": "caee1i3",
    "Central India": "incc1i3",
    "Central US": "uscc1i3",
    "China East": "cnee1i3",
    "China North": "cnnn1i3",
    "East Asia": "asee1i3",
    "East US 2": "usee2i3",
    "East US": "usee1i3",
    "Japan East": "jpee1i3",
    "Japan West": "jpww1i3",
    "North Central US": "usnc1i3",
    "North Europe": "eunn1i3",
    "South Central US": "ussc1i3",
    "Southeast Asia": "asse1i3",
    "South India": "inss1i3",
    "UK North": "gbnn1i3",
    "UK South 2": "gbss2i3",
    "West Central US": "uswc1i3",
    "West Europe": "euww1i3",
    "West India": "inww1i3",
    "West US 2": "usww2i3",
    "West US": "usww1i3",
}


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
        # Sort in theoretical version number order, not lexicographically
        versions.sort(key=lambda x: [int(ns) for ns in x.name.split('.')])
        width = len('{}'.format(len(versions)))
        for num, version in enumerate(versions):
            version_name = '{:0{}d}'.format(num, width)
            yield make_item(version_name, version.name, full_spec,
                            region_name, endpoint)


def make_item(version_name, urn_version, full_spec, region_name, endpoint):
    URN = ':'.join(full_spec[1:] + (urn_version,))
    pn_template = (
        'com.ubuntu.cloud:server:{}:amd64' if full_spec[2] == 'CentOS'
        else 'com.ubuntu.cloud:windows:{}:amd64')
    product_name = pn_template.format(full_spec[0])
    return Item(
        'com.ubuntu.cloud:released:azure',
        product_name,
        version_name, ITEM_NAMES[region_name], {
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
