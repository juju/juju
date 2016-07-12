from unittest import TestCase

from mock import (
    Mock,
    patch,
    )

from azure_image_streams import (
    get_azure_credentials,
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
