import logging
import re

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

from build_package import juju_series


CANONICAL = 'Canonical'
MS_VSTUDIO = 'MicrosoftVisualStudio'
MS_SERVER = 'MicrosoftWindowsServer'
UBUNTU_SERVER = 'UbuntuServer'
WINDOWS = 'Windows'
WINDOWS_SERVER = 'WindowsServer'
IMAGE_SPEC = [
    ('win81', MS_VSTUDIO, WINDOWS, 'Win8.1-Ent-N'),
    ('win10', MS_VSTUDIO, WINDOWS, 'Windows-10-N-x64'),
    ('win2012', MS_SERVER, WINDOWS_SERVER, '2012-Datacenter'),
    ('win2012r2', MS_SERVER, WINDOWS_SERVER, '2012-R2-Datacenter'),
    ('win2016', MS_SERVER, WINDOWS_SERVER, '2016-Datacenter'),
    ('win2016nano', MS_SERVER, WINDOWS_SERVER, '2016-Nano-Server'),
    ('centos7', 'OpenLogic', 'CentOS', '7.1'),
]


ITEM_NAMES = {
    "australiaeast": "auee1i3",
    "australiasoutheast": "ause1i3",
    "brazilsouth": "brss1i3",
    "canadacentral": "cacc1i3",
    "canadaeast": "caee1i3",
    "centralindia": "incc1i3",
    "centralus": "uscc1i3",
    "chinaeast": "cnee1i3",
    "chinanorth": "cnnn1i3",
    "eastasia": "asee1i3",
    "eastus2": "usee2i3",
    "eastus": "usee1i3",
    "japaneast": "jpee1i3",
    "japanwest": "jpww1i3",
    "northcentralus": "usnc1i3",
    "northeurope": "eunn1i3",
    "southcentralus": "ussc1i3",
    "southeastasia": "asse1i3",
    "southindia": "inss1i3",
    "uknorth": "gbnn1i3",
    "uksouth": "gbss1i3",
    "uksouth2": "gbss2i3",
    "ukwest": "gbww1i3",
    "westcentralus": "uswc1i3",
    "westeurope": "euww1i3",
    "westindia": "inww1i3",
    "westus2": "usww2i3",
    "westus": "usww1i3",
}


def logger():
    return logging.getLogger('azure_image_streams')


# Thorough investigation has not found an equivalent for these in the
# Azure-ARM image repository.
EXPECTED_MISSING = frozenset({
    ('12.04.2-LTS', '12.04.201212180'),
    ('16.04.0-LTS', '16.04.201611220'),
    ('16.04.0-LTS', '16.04.201611300'),
    })


class MissingImage(Exception):
    """Raised when an expected image is not present."""


class UnexpectedImage(Exception):
    """Raised when an image not expected is present."""


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


def parse_id(item_id):
    """Parse an old-style item-id to determine sku and version.

    From old-style ID, we ignore the first 32 chars.
    From "Ubuntu-14_04-LTS-amd64-server-20140416.1-en-us-30GB", we extract
    "14_04-LTS", "20140416" and "1"
    We convert to the sku 14.04.0-LTS and the version to 201404161.  (If there
    was no ".1", we'd use "0".)
    Note that for LTS only, the sku's version number always has a third digit
    (patchlevel).

    For 16.04.0-LTS, anything with beta in the id has the sku overriden to
    '16.04-beta'
    """
    match = re.match(
        '^.{32}__Ubuntu-(.*)-.*-server-(\d+)(\.(\d+))?(\-([^\d]*)\d+)?'
        '-.{2}-.{2}-\d+GB$', item_id)
    sku = match.group(1)
    sku = sku.replace('_', '.')
    # Prepare to manipulate the sku's version number.
    sku_sections = sku.split('-')
    if match.group(6) == 'beta' and len(sku_sections) > 1:
        sku_sections[1:] = ['beta']
    sku_num = sku_sections[0]
    sku_num_parts = sku_num.split('.')
    # foo-LTS always has a three-part version number, but for non-LTS, no third
    # digit should be added.
    if len(sku_num_parts) < 3 and sku_sections[1:] == ['LTS']:
        sku_num_parts.append('0')
    sku_num = '.'.join(sku_num_parts)
    sku = '-'.join([sku_num] + sku_sections[1:])
    version = '.'.join(sku_num_parts[0:2] + [match.group(2)])
    number = match.group(4)
    if number is not None:
        version += number
    else:
        version += '0'
    return sku, version


def arm_image_exists(client, location, full_spec):
    """Return True if the full_spec exists on Azure-ARM, else False."""
    try:
        client.virtual_machine_images.get(location, *full_spec)
    except CloudError as e:
        if e.message != 'Artifact: VMImage was not found.':
            raise
        return False
    else:
        return True


def convert_item_to_arm(item, urn, endpoint, region):
    """Return the ARM equivalent of an item, given a urn + endpoint."""
    data = dict(item.data)
    data.pop('crsn', None)
    data.update({'id': urn, 'endpoint': endpoint, 'region': region})
    return Item(item.content_id, item.product_name, item.version_name,
                item.item_name, data=data)


def sku_version_items(items):
    """Return a sorted list of tuples of (sku, version, item).

    The items' ids must support parse_id.
    """
    sort_items = []
    for item in items:
        sku, version = parse_id(item.data['id'])
        sort_items.append((sku, version, item))
    sort_items.sort()
    return sort_items


def convert_cloud_images_items(client, locations, items):
    """Convert cloud-images Azure data to Azure-ARM data."""
    arm_items = []
    endpoint = client.config.base_url
    location_map = dict((l.display_name, l.name) for l in locations)
    unknown_locations = set()
    for sku, version, item in sku_version_items(items):
        location_display_name = item.data['region']
        location = location_map.get(location_display_name)
        if location is None:
            unknown_locations.add(location_display_name)
            continue
        full_spec = (CANONICAL, UBUNTU_SERVER, sku, version)
        urn = ':'.join(full_spec)
        if not arm_image_exists(client, location, full_spec):
            if (sku, version) not in EXPECTED_MISSING:
                raise MissingImage('{} not in {}\n'.format(urn, location))
            continue
        if (sku, version) in EXPECTED_MISSING:
            raise UnexpectedImage(
                'Unexpectedly found {} in {}\n'.format(urn, location))
        arm_items.append(convert_item_to_arm(item, urn, endpoint, location))
    return arm_items, unknown_locations


def make_spec_items(client, full_spec, locations):
    """Return Items for all versions this spec in all Azure locations.

    full_spec is the spec to use for looking up versions.
    locations is a list of Azure Locations.
    """
    endpoint = client.config.base_url
    spec = full_spec[1:]
    location_versions = {}
    for location in locations:
        logger().debug('Retrieving image data in {}'.format(
            location.display_name))
        try:
            versions = client.virtual_machine_images.list(location.name,
                                                          *spec)
        except CloudError:
            template = 'Could not find {} {} {} in {location}'
            logger().warning(template.format(
                *spec, location=location.display_name))
            continue
        for version in versions:
            location_versions.setdefault(
                version.name, set()).add(location.name)
    lv2 = sorted(location_versions.items(), key=lambda x: [
        int(ns) for ns in x[0].split('.')])
    for num, (version, v_locations) in enumerate(lv2):
        # Sort in theoretical version number order, not lexicographically
        width = len('{}'.format(len(versions)))
        for location in v_locations:
            version_name = '{:0{}d}'.format(num, width)
            yield make_item(version_name, version, full_spec, location,
                            endpoint)


def make_item(version_name, urn_version, full_spec, location_name, endpoint,
              stream='released', item_version=None, release=None):
    """Make a simplestreams Item for a version.

    Version name is the simplestreams version_name.
    urn_version is the Azure version.name, used to generate the URN ID.
    full_spec is the spec that was used to list the versions.
    location_name is the Azure display name.
    endpoint is the URL used as an Azure endpoint.

    The item_name is looked up from ITEM_NAMES.
    """
    URN = ':'.join(full_spec[1:] + (urn_version,))
    product_name = 'com.ubuntu.cloud:server:{}:amd64'.format(full_spec[0])
    if release is None:
        release = full_spec[0]
    if item_version is None:
        item_version = full_spec[0]
    return Item(
        'com.ubuntu.cloud:{}:azure'.format(stream),
        product_name,
        version_name, ITEM_NAMES[location_name], {
            'arch': 'amd64',
            'virt': 'Hyper-V',
            'region': location_name,
            'id': URN,
            'label': 'release',
            'endpoint': endpoint,
            'release': release,
            'version': item_version,
            }
        )


class ItemList(mirrors.BasicMirrorWriter):
    """A class that can retrieve the items from a given url.

    Based on sstream-query.
    """

    @classmethod
    def items_from_url(cls, url):
        source = mirrors.UrlMirrorReader(url)
        target = cls()
        target.sync(source, 'com.ubuntu.cloud:released:azure.sjson')
        return target.items

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
    subscription_id, credentials = get_azure_credentials(all_credentials)
    sub_client = SubscriptionClient(credentials)
    client = ComputeManagementClient(credentials, subscription_id)
    locations = sub_client.subscriptions.list_locations(subscription_id)
    items = find_ubuntu_items(client, locations)
    items.extend(find_spec_items(client, locations))
    return items


def find_ubuntu_items(client, locations):
    """Make simplestreams Items for existing Azure images.

    All versions of all images matching IMAGE_SPEC will be returned.

    all_credentials is a dict of credentials in the credentials.yaml
    structure, used to create Azure credentials.
    """
    items = []
    for location in locations:
        skus = client.virtual_machine_images.list_skus(
            location.name, CANONICAL, UBUNTU_SERVER)
        for sku in skus:
            match = re.match(r'(\d\d\.\d\d)(\.\d+)?-?(.*)', sku.name)
            if match is None:
                logger().info('Skipping {}'.format(sku.name))
                continue
            tag = match.group(3)
            if tag in ('DAILY', 'DAILY-LTS'):
                stream = 'daily'
            elif tag in ('', 'LTS'):
                stream = 'released'
            else:
                logger().info('Skipping {}'.format(sku.name))
                continue
            minor_version = match.group(1)
            try:
                release = juju_series.get_name(minor_version)
            except KeyError:
                logger().warning("Can't find name for {}".format(release))
                continue
            full_spec = (minor_version, CANONICAL, UBUNTU_SERVER, sku.name)
            items.append(make_item(sku.name, 'latest', full_spec,
                                   location.name, client.config.base_url,
                                   stream=stream, release=release))
    return items


def find_spec_items(client, locations):
    """Make simplestreams Items for existing Azure images.

    All versions of all images matching IMAGE_SPEC will be returned.

    all_credentials is a dict of credentials in the credentials.yaml
    structure, used to create Azure credentials.
    """
    items = []
    for full_spec in IMAGE_SPEC:
        items.extend(make_spec_items(client, full_spec, locations))
    return items
