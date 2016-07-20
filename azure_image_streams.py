import logging
import re
import sys

# Tested on Azure 2.0 rc5 API
from azure.common.credentials import ServicePrincipalCredentials
from azure.mgmt.compute import (
    ComputeManagementClient,
    )
from azure.mgmt.resource.subscriptions import SubscriptionClient
from msrestazure.azure_exceptions import CloudError
from simplestreams.json2streams import (
    dict_to_item,
    Item,
    )
from simplestreams import mirrors
from simplestreams import util


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


def predict_urns(items, locations):
    location_map = dict((l.display_name, l.name) for l in locations)
    for item in items:
        sku, version = parse_id(item.data['id'])
        yield 'Canonical:UbuntuServer:{0}:{1}'.format(sku, version)


def parse_id(item_id):
    match = re.match(
        '^.{32}__Ubuntu-(.*)-.*-server-(\d+)(\.(\d+))?(\-.*)?-.{2}-.{2}-\d+GB$',
        item_id)
    sku = match.group(1)
    sku = sku.replace('_', '.')
    sku_sections = sku.split('-')
    sku_num = sku_sections[0]
    sku_num_parts = sku_num.split('.')
    # foo-LTS typically has a three-part version number, which foo typically
    # does not.
    if len(sku_num_parts) < 3 and len(sku_sections) > 1:
        sku_num_parts.append('0')
    sku_num = '.'.join(sku_num_parts)
    sku = '-'.join([sku_num] + sku_sections[1:])
    if sku == '16.04.0-LTS' and 'beta' in item_id:
        sku = '16.04-beta'
    version = '.'.join(sku_num_parts[0:2] + [match.group(2)])
    number = match.group(4)
    version = version.split('-')[0]
    if number is not None:
        version += number
    else:
        version += '0'
    return sku, version


def arm_item_exists(client, location, full_spec):
    try:
        result = client.virtual_machine_images.get(
            location, *full_spec)
    except CloudError as e:
        if e.message != 'Artifact: VMImage was not found.':
            raise
        return False
    else:
        return True


def make_arm_item(item, urn, endpoint):
    data = dict(item.data)
    data.pop('crsn', None)
    data.update({'id': urn, 'endpoint': endpoint})
    return Item(item.content_id, item.product_name, item.version_name,
                item.item_name, data=data)


def get_arm_items(client, items, locations):
    sort_items = []
    for item in items:
        sku, version = parse_id(item.data['id'])
        sort_items.append((sku, version, item))
    sort_items.sort()
    arm_items = []
    endpoint = client.config.base_url
    location_map = dict((l.display_name, l.name) for l in locations)
    unknown_locations = set()
    for sku, version, item in sort_items:
        location = location_map.get(item.data['region'])
        if location is None:
            unknown_locations.add(location)
            continue
        full_spec = ('Canonical', 'UbuntuServer', sku, version)
        urn = ':'.join(full_spec)
        if not arm_item_exists(client, location, full_spec):
            sys.stderr.write('{} not in {}\n'.format(urn, location))
            continue
        arm_items.append(make_arm_item(item, urn, endpoint))
    sys.stderr.write('Unknown locations: {}\n'.format(','.format(
        unknown_locations)))
    return arm_items


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


class ItemList(mirrors.BasicMirrorWriter):

    @classmethod
    def from_url(cls, url):
        source = mirrors.UrlMirrorReader(url)
        target = cls()
        target.sync(source, 'com.ubuntu.cloud:released:azure.sjson')
        return target

    def __init__(self):
        self.config = {}
        self.items = []

    def load_products(self, path, content_id):
        pass

    def insert_item(self, data, src, target, pedigree, contentsource):
        data = util.products_exdata(src, pedigree)
        self.items.append(dict_to_item(data))



def make_azure_items(all_credentials):
    """Make simplestreams Items for existing Azure images.

    All versions of all images matching IMAGE_SPEC will be returned.

    all_credentials is a dict of credentials in the credentials.yaml
    structure, used to create Azure credentials.
    """
    item_list = ItemList.from_url(
        'http://cloud-images.ubuntu.com/releases/streams/v1')
    subscription_id, credentials = get_azure_credentials(all_credentials)
    sub_client = SubscriptionClient(credentials)
    client = ComputeManagementClient(credentials, subscription_id)
    locations = sub_client.subscriptions.list_locations(subscription_id)
    items = get_arm_items(client, item_list.items, locations)
    for full_spec in IMAGE_SPEC:
        items.extend(make_spec_items(client, full_spec, locations))
    return items
