from unittest import TestCase

from mock import (
    call,
    Mock,
    patch,
    )

from azure_image_streams import (
    get_azure_credentials,
    get_image_versions,
    IMAGE_SPEC,
    make_item,
    )
from simplestreams.json2streams import Item


class TestGetAzureCredentials(TestCase):

    def test_get_azure_credentials(self):
        all_credentials = {'azure': {'credentials': {
            'application-id': 'application-id1',
            'application-password': 'password1',
            'subscription-id': 'subscription-id1',
            'tenant-id': 'tenant-id1',
            }}}
        with patch(
                'azure_image_streams.ServicePrincipalCredentials') as mock_spc:
            subscription_id, credentials = get_azure_credentials(
                all_credentials)
        self.assertEqual('subscription-id1', subscription_id)
        self.assertIs(mock_spc.return_value, credentials)
        mock_spc.assert_called_once_with(
            client_id='application-id1',
            secret='password1',
            subscription_id='subscription-id1',
            tenant='tenant-id1',
            )


class TestMakeItem(TestCase):

    def make_item(self, centos=False):
        version = Mock(location='usns')
        version.name = 'pete'
        offer = 'CentOS' if centos else 'bar'
        release = 'win95'
        full_spec = (release, 'foo', offer, 'baz')
        region_name = 'US Northsouth'
        endpoint = 'http://example.org'
        return make_item(version, full_spec, region_name, endpoint)

    def test_make_item(self):
        item = self.make_item()
        self.assertEqual(Item(
            'com.ubuntu.cloud:released:azure',
            'com.ubuntu.cloud:windows',
            'pete',
            'usns', {
                'arch': 'amd64',
                'virt': 'Hyper-V',
                'region': 'US Northsouth',
                'id': 'foo:bar:baz:pete',
                'label': 'release',
                'endpoint': 'http://example.org',
                'release': 'win95',
            }), item)

    def test_make_item_centos(self):
        item = self.make_item(centos=True)
        self.assertEqual(Item(
            'com.ubuntu.cloud:released:azure',
            'com.ubuntu.cloud:server:centos7:amd64',
            'pete',
            'usns', {
                'arch': 'amd64',
                'virt': 'Hyper-V',
                'region': 'US Northsouth',
                'id': 'foo:CentOS:baz:pete',
                'label': 'release',
                'endpoint': 'http://example.org',
                'release': 'win95',
            }), item)


class TestGetImageVersions(TestCase):

    def test_get_image_versions(self):
        client = Mock(spec=['config', 'virtual_machine_images'])
        version_1 = Mock()
        version_1.name = '1'
        version_2 = Mock()
        version_2.name = '2'
        client.virtual_machine_images.list.return_value = [version_1,
                                                           version_2]
        items = list(get_image_versions(client, 'region1', 'Region 1'))
        expected_items = []
        expected_calls = []
        for spec in IMAGE_SPEC:
            expected_calls.append(call('region1', *spec[1:]))
            expected_items.append(
                make_item(version_1, spec, 'Region 1', client.config.base_url))
            expected_items.append(
                make_item(version_2, spec, 'Region 1', client.config.base_url))
        self.assertEqual(expected_items, items)
        self.assertEqual(expected_calls,
                         client.virtual_machine_images.list.mock_calls)
