from contextlib import contextmanager
from unittest import TestCase

from mock import (
    call,
    Mock,
    patch,
    )
from msrestazure.azure_exceptions import CloudError

from azure_image_streams import (
    arm_image_exists,
    CANONICAL,
    convert_cloud_images_items,
    convert_item_to_arm,
    get_azure_credentials,
    IMAGE_SPEC,
    make_spec_items,
    MissingImage,
    make_item,
    make_azure_items,
    make_ubuntu_item,
    parse_id,
    UBUNTU_SERVER,
    UnexpectedImage,
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


def make_id(patch='_5', build_number='.4', lts=True, beta=False):
    sku_suffix = '-LTS' if lts else ''
    beta_suffix = '-beta256' if beta else ''
    variables = {
        'patch': patch,
        'sku_suffix': sku_suffix,
        'build_number': build_number,
        'beta_suffix': beta_suffix,
        }
    return (
        'b39f27a8b8c64d52b05eac6a62ebad85'
        '__Ubuntu-12_04{patch}{sku_suffix}-amd64-'
        'server-20140924{build_number}{beta_suffix}-en-us-30GB'
        ).format(**variables)


class TestParseID(TestCase):

    def test_parse_id(self):
        sku, version = parse_id(make_id())
        self.assertEqual('12.04.5-LTS', sku)
        self.assertEqual('12.04.201409244', version)

    def test_no_patchlevel(self):
        sku, version = parse_id(make_id(patch=''))
        self.assertEqual('12.04.0-LTS', sku)
        self.assertEqual('12.04.201409244', version)

    def test_no_patchlevel_not_lts(self):
        sku, version = parse_id(make_id(patch='', lts=False))
        self.assertEqual('12.04', sku)
        self.assertEqual('12.04.201409244', version)

    def test_beta(self):
        sku, version = parse_id(make_id(beta=True))
        self.assertEqual('12.04.5-beta', sku)
        self.assertEqual('12.04.201409244', version)

    def test_beta_no_patch(self):
        sku, version = parse_id(make_id(patch='', beta=True))
        self.assertEqual('12.04-beta', sku)
        self.assertEqual('12.04.201409244', version)

    def test_beta_not_lts(self):
        sku, version = parse_id(make_id(beta=True, lts=False))
        self.assertEqual('12.04.5', sku)
        self.assertEqual('12.04.201409244', version)

    def test_no_build_number(self):
        sku, version = parse_id(make_id(build_number=''))
        self.assertEqual('12.04.5-LTS', sku)
        self.assertEqual('12.04.201409240', version)


def force_missing(client):
    client.virtual_machine_images.get.side_effect = CloudError(
        Mock(), 'Artifact: VMImage was not found.')


class TestArmImageExists(TestCase):

    def test_image_exists(self):
        client = Mock()
        self.assertTrue(arm_image_exists(client, 'foo', ()))
        client.virtual_machine_images.get.assert_called_once_with('foo')

    def test_image_missing(self):
        client = Mock()
        force_missing(client)
        self.assertFalse(arm_image_exists(client, 'foo', ()))
        client.virtual_machine_images.get.assert_called_once_with('foo')

    def test_other_error(self):
        client = Mock()
        client.virtual_machine_images.get.side_effect = CloudError(
            Mock(), 'Other error')
        with self.assertRaises(CloudError):
            arm_image_exists(client, 'foo', ())
        client.virtual_machine_images.get.assert_called_once_with('foo')


def make_old_item(item_id=None, region=None):
    if region is None:
        region = 'Westeros'
    if item_id is None:
        item_id = make_id()
    return Item('aa', 'bb', 'cc', '99', {
        'id': item_id,
        'foo': 'bar',
        'endpoint': 'http://example.com/old',
        'region': region,
        })


class TestConvertItemToARM(TestCase):

    def test_convert_item_to_arm(self, item_id=None):
        item = make_old_item(item_id)
        arm_item = convert_item_to_arm(
            item, 'ww:xx:yy:zz', 'http://example.com/arm', 'westeros')
        self.assertEqual(arm_item, Item('aa', 'bb', 'cc', '99', {
            'id': 'ww:xx:yy:zz',
            'foo': 'bar',
            'endpoint': 'http://example.com/arm',
            'region': 'westeros',
            }))

    def test_discard_crsn(self):
        item = make_old_item()
        item.data['crsn'] = 'asdf'
        arm_item = convert_item_to_arm(
            item, 'ww:xx:yy:zz', 'http://example.com/arm', 'westeros')
        self.assertNotIn('crsn', arm_item.data)


def make_item_expected(item_id=None, region=None, endpoint=None):
    if endpoint is None:
        endpoint = 'asdf'
    old_item = make_old_item(item_id=item_id, region=region)
    sku, version = parse_id(old_item.data['id'])
    full_spec = (CANONICAL, UBUNTU_SERVER, sku, version)
    urn = ':'.join(full_spec)
    arm_region = old_item.data['region'].lower().replace(' ', '')
    expected_item = convert_item_to_arm(old_item, urn, endpoint, arm_region)
    return old_item, full_spec, expected_item


class TestConvertCloudImagesItems(TestCase):

    def make_locations_client(self, expected_item):
        locations = [mock_location('westeros', 'Westeros')]
        client = Mock()
        client.config.base_url = expected_item.data['endpoint']
        return locations, client

    def test_convert_cloud_images_items(self):
        old_item, full_spec, expected_item = make_item_expected()
        locations, client = self.make_locations_client(expected_item)
        arm_items, unknown_locations = convert_cloud_images_items(
            client, locations, [old_item])
        client.virtual_machine_images.get.assert_called_once_with(
            'westeros', *full_spec)
        self.assertEqual([
            expected_item], arm_items)
        self.assertEqual(set(), unknown_locations)

    def test_unknown_location(self):
        old_item = make_old_item()
        locations = []
        client = Mock()
        arm_items, unknown_locations = convert_cloud_images_items(
            client, locations, [old_item])
        self.assertEqual([], arm_items)
        self.assertEqual({'Westeros'}, unknown_locations)

    def test_unexpected(self):
        old_item, full_spec, expected_item = make_item_expected(
            item_id='b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-12_04_2-LTS'
            '-amd64-server-20121218-en-us-30GB')
        locations, client = self.make_locations_client(expected_item)
        with self.assertRaises(UnexpectedImage):
            convert_cloud_images_items(client, locations, [old_item])

    def test_missing_image(self):
        old_item, full_spec, expected_item = make_item_expected()
        locations, client = self.make_locations_client(expected_item)
        force_missing(client)
        with self.assertRaises(MissingImage):
            convert_cloud_images_items(client, locations, [old_item])
        client.virtual_machine_images.get.assert_called_once_with(
            'westeros', *full_spec)


class TestMakeItem(TestCase):

    def make_item(self, centos=False):
        offer = 'CentOS' if centos else 'bar'
        release = 'centos7' if centos else 'win95'
        full_spec = (release, 'foo', offer, 'baz')
        region_name = 'canadaeast'
        endpoint = 'http://example.org'
        return make_item('1', 'pete', full_spec, region_name, endpoint)

    def test_make_item(self):
        item = self.make_item()
        self.assertEqual(Item(
            'com.ubuntu.cloud:released:azure',
            'com.ubuntu.cloud:server:win95:amd64',
            '1',
            'caee1i3', {
                'arch': 'amd64',
                'virt': 'Hyper-V',
                'region': 'canadaeast',
                'id': 'foo:bar:baz:pete',
                'label': 'release',
                'endpoint': 'http://example.org',
                'release': 'win95',
                'version': 'win95',
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
                'region': 'canadaeast',
                'id': 'foo:CentOS:baz:pete',
                'label': 'release',
                'endpoint': 'http://example.org',
                'release': 'centos7',
                'version': 'centos7',
            }), item)


def mock_compute_client(versions):
    client = Mock(spec=['config', 'virtual_machine_images'])
    client.virtual_machine_images.list.return_value = [
        mock_version(v) for v in versions]
    client.virtual_machine_images.list_skus.return_value = [
        mock_sku('12.04.2-LTS'),
        ]
    client.config.base_url = 'http://example.com/arm'

    return client


def mock_version(name):
    version = Mock()
    version.name = name
    return version


def mock_location(name, display_name):
    location = Mock(display_name=display_name)
    location.name = name
    return location


def mock_sku(name):
    sku = Mock()
    sku.name = name
    return sku


def make_expected(client, versions, specs):
    expected_items = []
    expected_calls = []
    for spec in specs:
        expected_calls.append(call('canadaeast', *spec[1:]))
        for num, version in enumerate(versions):
            expected_items.append(
                make_item(str(num), version, spec, 'canadaeast',
                          client.config.base_url))
    return expected_calls, expected_items


class TestMakeSpecItems(TestCase):

    def test_make_spec_items(self):
        client = mock_compute_client(['1', '2'])
        locations = [mock_location('canadaeast', 'Canada East')]
        items = list(make_spec_items(client, IMAGE_SPEC[0], locations))
        expected_calls, expected_items = make_expected(
            client, ['1', '2'], [IMAGE_SPEC[0]])
        self.assertEqual(expected_items, items)
        self.assertEqual(expected_calls,
                         client.virtual_machine_images.list.mock_calls)


class TestMakeAzureItems(TestCase):

    @contextmanager
    def mai_cxt(self, location, client, ubuntu_items):
        with mock_spc_cxt():
            with patch('azure_image_streams.SubscriptionClient') as sc_mock:
                subscriptions_mock = sc_mock.return_value.subscriptions
                subscriptions_mock.list_locations.return_value = [location]
                with patch(
                        'azure_image_streams.ComputeManagementClient'
                        ) as cmc_mock:
                    cmc_mock.return_value = client
                    with patch('azure_image_streams.ItemList.items_from_url',
                               return_value=ubuntu_items):
                        with patch('sys.stderr'):
                            yield

    def test_make_azure_items(self):
        all_credentials = make_all_credentials()
        client = mock_compute_client(['3'])
        expected_calls, expected_items = make_expected(client, ['3'],
                                                       IMAGE_SPEC)
        location = mock_location('canadaeast', 'Canada East')
        expected_items.insert(
            0, make_item('12.04.2-LTS', 'latest', (
                '12.04', CANONICAL, UBUNTU_SERVER, '12.04.2-LTS'
                ), 'canadaeast', client.config.base_url, release='precise'))
        with self.mai_cxt(location, client, []):
            items = make_azure_items(all_credentials)
        self.assertEqual(expected_items, items)

    def test_make_azure_items_no_ubuntu(self):
        all_credentials = make_all_credentials()
        client = mock_compute_client(['3'])
        client.virtual_machine_images.list_skus.return_value = []
        expected_calls, expected_items = make_expected(client, ['3'],
                                                       IMAGE_SPEC)
        location = mock_location('canadaeast', 'Canada East')
        with self.mai_cxt(location, client, []):
            items = make_azure_items(all_credentials)
        self.assertEqual(expected_items, items)


class TestMakeUbuntuItem(TestCase):

    def make_item(self, full_version='12.04.5-LTS', daily=False):
        stream = 'daily' if daily else 'released'
        return make_item(full_version, 'latest', (
            '12.04', CANONICAL, UBUNTU_SERVER, full_version,
            ), 'canadaeast', 'http://example.com', release='precise',
            stream=stream)

    def test_make_ubuntu_item(self):
        item = make_ubuntu_item('http://example.com', 'canadaeast',
                                '12.04.5-LTS')
        self.assertEqual(item, self.make_item())

    def test_no_lts(self):
        item = make_ubuntu_item('http://example.com', 'canadaeast',
                                '12.04.5')
        self.assertEqual(item, self.make_item('12.04.5'))

    def test_daily(self):
        item = make_ubuntu_item('http://example.com', 'canadaeast',
                                '12.04.5-DAILY')
        self.assertEqual(item.content_id, 'com.ubuntu.cloud:daily:azure')

    def test_daily_lts(self):
        item = make_ubuntu_item('http://example.com', 'canadaeast',
                                '12.04.5-DAILY-LTS')
        self.assertEqual(item.content_id, 'com.ubuntu.cloud:daily:azure')

    def test_unknown_tag(self):
        item = make_ubuntu_item('http://example.com', 'canadaeast',
                                '12.04.5-FOOBAR')
        self.assertIs(item, None)

    def test_not_a_version(self):
        item = make_ubuntu_item('http://example.com', 'canadaeast',
                                '12.q.5')
        self.assertIs(item, None)

    def test_xenial(self):
        item = make_ubuntu_item('http://example.com', 'canadaeast',
                                '16.04.5-LTS')
        self.assertEqual('xenial', item.data['release'])

    def test_version(self):
        item = make_ubuntu_item('http://example.com', 'canadaeast',
                                '16.04.5-LTS')
        self.assertEqual('16.04', item.data['version'])
        self.assertIn(':16.04:', item.product_name)
