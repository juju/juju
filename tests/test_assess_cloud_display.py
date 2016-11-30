from tests import TestCase

from mock import patch
import yaml

from fakejuju import fake_juju_client
from assess_cloud_display import (
    get_clouds,
    remove_display_attributes,
    )
from utility import JujuAssertionError


class TestRemoveDisplayAttributes(TestCase):

    def test_remove_display_attributes(self):
        cloud = {
            'defined': 'local',
            'description': 'Openstack Cloud',
            'type': 'openstack',
            }
        remove_display_attributes(cloud)
        self.assertEqual(cloud, {'type': 'openstack'})

    def test_remove_display_attributes_bad_defined(self):
        with self.assertRaises(JujuAssertionError):
            remove_display_attributes({'defined': 'foo'})

    def test_remove_display_attributes_bad_description(self):
        with self.assertRaises(JujuAssertionError):
            remove_display_attributes({
                'defined': 'local',
                'description': 'bar',
                'type': 'openstack',
                })


class TestGetClouds(TestCase):

    def get_clouds(self, cloud_dict):
        client = fake_juju_client()
        with patch.object(client._backend, 'get_juju_output',
                          return_value=yaml.safe_dump(cloud_dict)) as gjo_mock:
            result = get_clouds(client)
        gjo_mock.assert_called_once_with(
            'clouds', ('--format', 'yaml'), frozenset(['migration']), 'foo',
            None, user_name=None)
        return result

    def test_get_clouds_empty(self):
        cloud_dict = {}
        self.assertEqual(self.get_clouds(cloud_dict), {})

    def test_get_clouds_strips_builtin(self):
        cloud_dict = {'localhost1': {'defined': 'built-in'}}
        self.assertEqual(self.get_clouds(cloud_dict), {})

    def test_get_clouds_strips_defined_description(self):
        cloud_dict = {'localhost1': {
            'defined': 'local',
            'type': 'maas',
            'description': 'Metal As A Service',
            }}
        self.assertEqual(self.get_clouds(cloud_dict),
                         {'localhost1': {'type': 'maas'}})
