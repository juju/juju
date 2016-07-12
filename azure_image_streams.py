import logging
import yaml

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

from make_aws_image_streams import (
    get_parameters,
    make_aws_items,
    write_juju_streams,
    )


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
    }
    endpoint = client.config.base_url
    for release, spec in image_spec.items():
        try:
            versions = client.virtual_machine_images.list(region, *spec)
        except CloudError:
            logging.warning('Could not find {} {} {} in {region}'.format(*spec,
                            region=region))
            continue
        for version in versions:
            yield make_item(version, spec, release, region_name, endpoint)


def make_item(version, spec, release, region_name, endpoint):
    URN = ':'.join(spec + (version.name,))
    product_name = (
        'com.ubuntu.cloud:server:centos7:amd64' if spec[1] == 'CentOS'
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
            'release': release,
            }
        )


def make_azure_items(all_credentials):
    subscription_id, credentials = get_azure_credentials(all_credentials)
    sub_client = azure.mgmt.resource.subscriptions.SubscriptionClient(
        credentials)
    client = ComputeManagementClient(credentials, subscription_id)
    items = []
    for region in sub_client.subscriptions.list_locations(subscription_id):
        items.extend(get_image_versions(
            client, region.name, region.display_name))
    return items


def write_streams(items, out_dir):
    updated = util.timestamp()
    data = {'updated': updated, 'datatype': 'image-ids'}
    trees = items2content_trees(items, data)
    write_juju_streams(out_dir, trees, updated, [
        'path', 'sha256', 'md5', 'size', 'virt', 'root_store'])


def main():
    streams, creds_filename = get_parameters()
    with open(creds_filename) as creds_file:
        all_credentials = yaml.safe_load(creds_file)['credentials']
    items = make_azure_items(all_credentials)
    items.extend(make_aws_items(all_credentials))
    write_streams(items, 'outdir')


if __name__ == '__main__':
    main()
