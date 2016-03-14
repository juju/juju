from unittest import TestCase

from mock import (
    Mock,
    patch,
    )

from make_aws_image_streams import (
    is_china,
    iter_region_connection,
    )


class TestIsChina(TestCase):

    def test_is_china(self):
        region = Mock()
        region.endpoint = 'foo.amazonaws.com.cn'
        self.assertIs(True, is_china(region))
        region.endpoint = 'foo.amazonaws.com'
        self.assertIs(False, is_china(region))


class IterRegionConnection(TestCase):

    def test_iter_region_connection(self):
        east = Mock(endpoint='east-end')
        west = Mock(endpoint='west-end')
        east.name = 'east-name'
        west.name = 'west-name'
        aws = {}
        with patch('make_aws_image_streams.ec2.regions',
                   return_value=[east, west]) as regions_mock:
            connections = [x for x in iter_region_connection(aws, None)]
        regions_mock.assert_called_once_with()
        self.assertEqual(
            [east.connect.return_value, west.connect.return_value],
            connections)
        east.connect.assert_called_once_with(**aws)
        west.connect.assert_called_once_with(**aws)

    def test_gov_region(self):
        east = Mock(endpoint='east-end')
        gov = Mock(endpoint='west-end')
        east.name = 'east-name'
        gov.name = 'foo-us-gov-bar'
        aws = {}
        with patch('make_aws_image_streams.ec2.regions',
                   return_value=[east, gov]) as regions_mock:
            connections = [x for x in iter_region_connection(aws, None)]
        regions_mock.assert_called_once_with()
        self.assertEqual(
            [east.connect.return_value], connections)
        east.connect.assert_called_once_with(**aws)
        self.assertEqual(0, gov.connect.call_count)

    def test_china_region(self):
        east = Mock(endpoint='east-end')
        west = Mock(endpoint='west-end.amazonaws.com.cn')
        east.name = 'east-name'
        west.name = 'west-name'
        aws = {'name': 'aws'}
        aws_cn = {'name': 'aws-cn'}
        with patch('make_aws_image_streams.ec2.regions',
                   return_value=[east, west]) as regions_mock:
            connections = [x for x in iter_region_connection(aws, aws_cn)]
        regions_mock.assert_called_once_with()
        self.assertEqual(
            [east.connect.return_value, west.connect.return_value],
            connections)
        east.connect.assert_called_once_with(**aws)
        west.connect.assert_called_once_with(**aws_cn)
