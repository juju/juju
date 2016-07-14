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
    """Return the subscription_id and credentials for Azure.

    Takes a dict where key is the cloud name, expected to be formatted like
    cloud-city's credentials.
    """
    azure_dict = all_credentials['azure']['credentials']
    subscription_id = azure_dict['subscription-id']
    return subscription_id, ServicePrincipalCredentials(
        client_id=azure_dict['application-id'],
        secret=azure_dict['application-password'],
        tenant=azure_dict['tenant-id'],
        subscription_id=azure_dict['subscription-id'],
        )


def make_spec_items(client, full_spec, locations):
    """Return Items for all versions this spec in all Azure locations.

    full_spec is the spec to use for looking up versions.
    locations is a list of Azure Locations.
    """
    endpoint = client.config.base_url
    spec = full_spec[1:]
    location_versions = {}
    for location in locations:
        logging.info('Retrieving image data in {}'.format(
            location.display_name))
        try:
            versions = client.virtual_machine_images.list(location.name,
                                                          *spec)
        except CloudError:
            template = 'Could not find {} {} {} in {location}'
            logging.warning(template.format(
                *spec, location=location.display_name))
            continue
        for version in versions:
            location_versions.setdefault(
                version.name, set()).add(location.display_name)
    lv2 = sorted(location_versions.items(), key=lambda x: [
        int(ns) for ns in x[0].split('.')])
    for num, (version, v_locations) in enumerate(lv2):
        # Sort in theoretical version number order, not lexicographically
        width = len('{}'.format(len(versions)))
        for location in v_locations:
            version_name = '{:0{}d}'.format(num, width)
            yield make_item(version_name, version, full_spec, location,
                            endpoint)


def make_item(version_name, urn_version, full_spec, location_name, endpoint):
    """Make a simplestreams Item for a version.

    Version name is the simplestreams version_name.
    urn_version is the Azure version.name, used to generate the URN ID.
    full_spec is the spec that was used to list the versions.
    location_name is the Azure display name.
    endpoint is the URL used as an Azure endpoint.

    The item_name is looked up from ITEM_NAMES.
    """
    URN = ':'.join(full_spec[1:] + (urn_version,))
    pn_template = (
        'com.ubuntu.cloud:server:{}:amd64' if full_spec[2] == 'CentOS'
        else 'com.ubuntu.cloud:windows:{}:amd64')
    product_name = pn_template.format(full_spec[0])
    return Item(
        'com.ubuntu.cloud:released:azure',
        product_name,
        version_name, ITEM_NAMES[location_name], {
            'arch': 'amd64',
            'virt': 'Hyper-V',
            'region': location_name,
            'id': URN,
            'label': 'release',
            'endpoint': endpoint,
            'release': full_spec[0],
            }
        )


def make_azure_items(all_credentials):
    """Make simplestreams Items for existing Azure images.

    All versions of all images matching IMAGE_SPEC will be returned.

    all_credentials is a dict of credentials in the credentials.yaml
    structure, used to create Azure credentials.
    """
    subscription_id, credentials = get_azure_credentials(all_credentials)
    sub_client = SubscriptionClient(credentials)
    client = ComputeManagementClient(credentials, subscription_id)
    items = []
    locations = sub_client.subscriptions.list_locations(subscription_id)
    for full_spec in IMAGE_SPEC:
        items.extend(make_spec_items(client, full_spec, locations))
    return items
