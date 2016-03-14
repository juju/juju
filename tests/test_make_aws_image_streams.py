from datetime import datetime
import json
import os
from StringIO import StringIO
from unittest import TestCase

from mock import (
    Mock,
    patch,
    )

from make_aws_image_streams import (
    is_china,
    iter_centos_images,
    iter_region_connection,
    get_parameters,
    make_aws_credentials,
    make_item,
    write_streams,
    )
from utils import temp_dir


class TestIsChina(TestCase):

    def test_is_china(self):
        region = Mock()
        region.endpoint = 'foo.amazonaws.com.cn'
        self.assertIs(True, is_china(region))
        region.endpoint = 'foo.amazonaws.com'
        self.assertIs(False, is_china(region))


def make_mock_region(stem, name=None, endpoint=None):
    if endpoint is None:
        endpoint = '{}-end'.format(stem)
    region = Mock(endpoint=endpoint)
    if name is None:
        name = '{}-name'.format(stem)
    region.name = name
    return region


class IterRegionConnection(TestCase):

    def test_iter_region_connection(self):
        east = make_mock_region('east')
        west = make_mock_region('west')
        aws = {}
        with patch('make_aws_image_streams.ec2.regions', autospec=True,
                   return_value=[east, west]) as regions_mock:
            connections = [x for x in iter_region_connection(aws, None)]
        regions_mock.assert_called_once_with()
        self.assertEqual(
            [east.connect.return_value, west.connect.return_value],
            connections)
        east.connect.assert_called_once_with(**aws)
        west.connect.assert_called_once_with(**aws)

    def test_gov_region(self):
        east = make_mock_region('east')
        gov = make_mock_region('west', name='foo-us-gov-bar')
        aws = {}
        with patch('make_aws_image_streams.ec2.regions', autospec=True,
                   return_value=[east, gov]) as regions_mock:
            connections = [x for x in iter_region_connection(aws, None)]
        regions_mock.assert_called_once_with()
        self.assertEqual(
            [east.connect.return_value], connections)
        east.connect.assert_called_once_with(**aws)
        self.assertEqual(0, gov.connect.call_count)

    def test_china_region(self):
        east = make_mock_region('east')
        west = make_mock_region('west', endpoint='west-end.amazonaws.com.cn')
        east.name = 'east-name'
        west.name = 'west-name'
        aws = {'name': 'aws'}
        aws_cn = {'name': 'aws-cn'}
        with patch('make_aws_image_streams.ec2.regions', autospec=True,
                   return_value=[east, west]) as regions_mock:
            connections = [x for x in iter_region_connection(aws, aws_cn)]
        regions_mock.assert_called_once_with()
        self.assertEqual(
            [east.connect.return_value, west.connect.return_value],
            connections)
        east.connect.assert_called_once_with(**aws)
        west.connect.assert_called_once_with(**aws_cn)


class IterCentosImages(TestCase):

    def test_iter_centos_images(self):
        aws = {'name': 'aws'}
        aws_cn = {'name': 'aws-cn'}
        east_imgs = ['east-1', 'east-2']
        west_imgs = ['west-1', 'west-2']
        east_conn = Mock()
        east_conn.get_all_images.return_value = east_imgs
        west_conn = Mock()
        west_conn.get_all_images.return_value = west_imgs
        with patch('make_aws_image_streams.iter_region_connection',
                   return_value=[east_conn, west_conn],
                   autospec=True) as irc_mock:
            imgs = list(iter_centos_images(aws, aws_cn))
        self.assertEqual(east_imgs + west_imgs, imgs)
        irc_mock.assert_called_once_with(aws, aws_cn)
        east_conn.get_all_images.assert_called_once_with(filters={
            'owner_alias': 'aws-marketplace',
            'product_code': 'aw0evgkw8e5c1q413zgy5pjce',
            })
        west_conn.get_all_images.assert_called_once_with(filters={
            'owner_alias': 'aws-marketplace',
            'product_code': 'aw0evgkw8e5c1q413zgy5pjce',
            })


class TestMakeAWSCredentials(TestCase):

    def test_happy_path(self):
        aws_credentials = make_aws_credentials({'credentials': {
            'access-key': 'foo',
            'secret-key': 'bar',
            }})
        self.assertEqual({
            'aws_access_key_id': 'foo',
            'aws_secret_access_key': 'bar',
            }, aws_credentials)

    def test_no_credentials(self):
        with self.assertRaisesRegexp(LookupError, 'No credentials found!'):
            make_aws_credentials({})

    def test_multiple_credentials(self):
        # If multiple credentials are present, an arbitrary credential will be
        # used.
        aws_credentials = make_aws_credentials({
            'credentials-1': {
                'access-key': 'foo',
                'secret-key': 'bar',
                },
            'credentials-2': {
                'access-key': 'baz',
                'secret-key': 'qux',
                },
            })
        self.assertIn(aws_credentials, [
            {'aws_access_key_id': 'foo', 'aws_secret_access_key': 'bar'},
            {'aws_access_key_id': 'baz', 'aws_secret_access_key': 'qux'},
            ])


def make_mock_image(region_name='bar'):
    image = Mock(virtualization_type='baz', id='qux',
                 root_device_type='quxx', architecture='x86_64')
    image.name = 'CentOS Linux 7 foo'
    image.region.endpoint = 'foo'
    image.region.name = region_name
    return image


class TetMakeItem(TestCase):

    def test_happy_path(self):
        image = make_mock_image()
        now = datetime(2001, 02, 03)
        item = make_item(image, now)
        self.assertEqual(item.content_id, 'com.ubuntu.cloud.released:aws')
        self.assertEqual(item.product_name,
                         'com.ubuntu.cloud:server:centos7:amd64')
        self.assertEqual(item.item_name, 'bar')
        self.assertEqual(item.version_name, '20010203')
        self.assertEqual(item.data, {
            'endpoint': 'https://foo',
            'region': 'bar',
            'arch': 'amd64',
            'os': 'centos',
            'virt': 'baz',
            'id': 'qux',
            'version': 'centos7',
            'label': 'release',
            'release': 'centos7',
            'release_codename': 'centos7',
            'release_title': 'Centos 7',
            'root_store': 'quxx',
            })

    def test_china(self):
        image = make_mock_image()
        image.region.endpoint = 'foo.amazonaws.com.cn'
        now = datetime(2001, 02, 03)
        item = make_item(image, now)
        self.assertEqual(item.content_id, 'com.ubuntu.cloud.released:aws-cn')
        self.assertEqual(item.data['endpoint'], 'https://foo.amazonaws.com.cn')

    def test_not_x86_64(self):
        image = make_mock_image()
        image.architecture = 'ppc128'
        now = datetime(2001, 02, 03)
        with self.assertRaisesRegexp(ValueError,
                                     'Architecture is "ppc128", not'
                                     ' "x86_64".'):
            make_item(image, now)

    def test_not_centos_7(self):
        image = make_mock_image()
        image.name = 'CentOS Linux 8'
        now = datetime(2001, 02, 03)
        with self.assertRaisesRegexp(ValueError,
                                     'Name "CentOS Linux 8" does not begin'
                                     ' with "CentOS Linux 7".'):
            make_item(image, now)


class TestGetParameters(TestCase):

    def test_happy_path(self):
        with patch.dict(os.environ, {'JUJU_DATA': 'foo'}):
            streams, creds_filename = get_parameters(['bar'])
        self.assertEqual(creds_filename, 'foo/credentials.yaml')
        self.assertEqual(streams, 'bar')

    def test_no_juju_data(self):
        stderr = StringIO()
        with self.assertRaises(SystemExit):
            with patch('sys.stderr', stderr):
                get_parameters(['bar'])
        self.assertEqual(
            stderr.getvalue(),
            'JUJU_DATA must be set to a directory containing'
            ' credentials.yaml.\n')


def load_json(parent, filename):
    with open(os.path.join(parent, 'streams', 'v1', filename)) as f:
        return json.load(f)


class TestWriteStreams(TestCase):

    def test_write_streams(self):
        now = datetime(2001, 02, 03)
        credentials = {'name': 'aws'}
        china_credentials = {'name': 'aws-cn'}
        east_conn = Mock()
        east_image = make_mock_image(region_name='east')
        east_conn.get_all_images.return_value = [east_image]
        west_conn = Mock()
        west_image = make_mock_image(region_name='west')
        west_conn.get_all_images.return_value = [west_image]
        with temp_dir() as streams:
            with patch('make_aws_image_streams.iter_region_connection',
                       return_value=[east_conn, west_conn],
                       autospec=True) as irc_mock:
                with patch('make_aws_image_streams.timestamp',
                           return_value='now'):
                    with patch('sys.stderr'):
                        write_streams(credentials, china_credentials, now,
                                      streams)
            index = load_json(streams, 'index.json')
            index2 = load_json(streams, 'index2.json')
            releases = load_json(streams, 'com.ubuntu.cloud.released-aws.json')
            irc_mock.assert_called_once_with(credentials, china_credentials)
        self.assertEqual(
            {'format': 'index:1.0', 'updated': 'now', 'index': {}}, index)
        self.assertEqual(
            {'format': 'index:1.0', 'updated': 'now', 'index': {
                'com.ubuntu.cloud.released:aws': {
                    'format': 'products:1.0',
                    'updated': 'now',
                    'datatype': 'image-ids',
                    'path': 'streams/v1/com.ubuntu.cloud.released-aws.json',
                    'products': ['com.ubuntu.cloud:server:centos7:amd64'],
                    }
                }}, index2)
        expected = {
            'content_id': 'com.ubuntu.cloud.released:aws',
            'format': 'products:1.0',
            'updated': 'now',
            'datatype': 'image-ids',
            'products': {'com.ubuntu.cloud:server:centos7:amd64': {
                'root_store': 'quxx',
                'endpoint': 'https://foo',
                'arch': 'amd64',
                'release_title': 'Centos 7',
                'label': 'release',
                'release_codename': 'centos7',
                'version': 'centos7',
                'virt': 'baz',
                'release': 'centos7',
                'os': 'centos',
                'id': 'qux',
                'versions': {'20010203': {
                    'items': {
                        'west': {'region': 'west'},
                        'east': {'region': 'east'},
                        }
                    }},
                }},
            }
        self.assertEqual(releases, expected)
