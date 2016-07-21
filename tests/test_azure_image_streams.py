from contextlib import contextmanager
from unittest import TestCase

from mock import (
    call,
    Mock,
    patch,
    )

from azure_image_streams import (
    get_azure_credentials,
    make_spec_items,
    IMAGE_SPEC,
    make_item,
    make_azure_items,
    )
from simplestreams.json2streams import Item


def make_all_credentials():
    return {'azure': {'credentials': {
            'application-id': 'application-id1',
            'application-password': 'password1',
            'subscription-id': 'subscription-id1',
            'tenant-id': 'tenant-id1',
            }}}


@contextmanager
def mock_spc_cxt():
    with patch(
            'azure_image_streams.ServicePrincipalCredentials') as mock_spc:
        yield mock_spc
    mock_spc.assert_called_once_with(
        client_id='application-id1',
        secret='password1',
        subscription_id='subscription-id1',
        tenant='tenant-id1',
        )


class TestGetAzureCredentials(TestCase):

    def test_get_azure_credentials(self):
        all_credentials = make_all_credentials()
        with mock_spc_cxt() as mock_spc:
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
        offer = 'CentOS' if centos else 'bar'
        release = 'centos7' if centos else 'win95'
        full_spec = (release, 'foo', offer, 'baz')
        region_name = 'Canada East'
        endpoint = 'http://example.org'
        return make_item('1', 'pete', full_spec, region_name, endpoint)

    def test_make_item(self):
        item = self.make_item()
        self.assertEqual(Item(
            'com.ubuntu.cloud:released:azure',
            'com.ubuntu.cloud:windows:win95:amd64',
            '1',
            'caee1i3', {
                'arch': 'amd64',
                'virt': 'Hyper-V',
                'region': 'Canada East',
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
            '1',
            'caee1i3', {
                'arch': 'amd64',
                'virt': 'Hyper-V',
                'region': 'Canada East',
                'id': 'foo:CentOS:baz:pete',
                'label': 'release',
                'endpoint': 'http://example.org',
                'release': 'centos7',
            }), item)


def mock_compute_client(versions):
    client = Mock(spec=['config', 'virtual_machine_images'])
    client.virtual_machine_images.list.return_value = [
        mock_version(v) for v in versions]
    return client


def mock_version(name):
    version = Mock()
    version.name = name
    return version


def mock_location(name, display_name):
    location = Mock(display_name=display_name)
    location.name = name
    return location


def make_expected(client, versions, specs):
    expected_items = []
    expected_calls = []
    for spec in specs:
        expected_calls.append(call('region1', *spec[1:]))
        for num, version in enumerate(versions):
            expected_items.append(
                make_item(str(num), version, spec, 'Canada East',
                          client.config.base_url))
    return expected_calls, expected_items


class TestMakeSpecItems(TestCase):

    def test_make_spec_items(self):
        client = mock_compute_client(['1', '2'])
        locations = [mock_location('region1', 'Canada East')]
        items = list(make_spec_items(client, IMAGE_SPEC[0], locations))
        expected_calls, expected_items = make_expected(
            client, ['1', '2'], [IMAGE_SPEC[0]])
        self.assertEqual(expected_items, items)
        self.assertEqual(expected_calls,
                         client.virtual_machine_images.list.mock_calls)


class TestMakeAzureItems(TestCase):

    def test_make_azure_items(self):
        all_credentials = make_all_credentials()
        client = mock_compute_client(['3'])
        expected_calls, expected_items = make_expected(client, ['3'],
                                                       IMAGE_SPEC)
        location = mock_location('canadaeast', 'Canada East')
        with mock_spc_cxt():
            with patch('azure_image_streams.SubscriptionClient') as sc_mock:
                subscriptions_mock = sc_mock.return_value.subscriptions
                subscriptions_mock.list_locations.return_value = [location]
                with patch(
                        'azure_image_streams.ComputeManagementClient'
                        ) as cmc_mock:
                    cmc_mock.return_value = client
                    items = make_azure_items(all_credentials)
        self.assertEqual(expected_items, items)
