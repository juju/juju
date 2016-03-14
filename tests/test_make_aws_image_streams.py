from unittest import TestCase

from mock import (
    Mock,
    patch,
    )

from make_aws_image_streams import (
    is_china,
    iter_centos_images,
    iter_region_connection,
    make_aws_credentials,
    )


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
