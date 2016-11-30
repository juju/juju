from contextlib import contextmanager
from collections import OrderedDict

from mock import (
    call,
    patch,
    )
import yaml

from fakejuju import fake_juju_client
from assess_cloud_display import (
    assess_clouds,
    assess_show_cloud,
    get_clouds,
    remove_display_attributes,
    )
from tests import TestCase
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


@contextmanager
def override_clouds(client, cloud_dict):
    with patch.object(client._backend, 'get_juju_output',
                      return_value=yaml.safe_dump(cloud_dict)) as gjo_mock:
        yield
    expected_name, expected_args, expected_kwargs = gjo_call(
        client, 'clouds', ('--format', 'yaml'))
    gjo_mock.assert_called_once_with(*expected_args, **expected_kwargs)


def gjo_call(client, cmd, args):
    timeout = None
    return call(
        cmd, args, client.used_feature_flags, client.env.juju_home, timeout,
        user_name=client.env.user_name)


@contextmanager
def override_show_cloud(test_case, client, cloud_dict):
    results = [yaml.safe_dump(c) for c in cloud_dict.values()]
    with patch.object(client._backend, 'get_juju_output',
                      side_effect=results) as gjo_mock:
        yield
    test_case.assertItemsEqual([
        gjo_call(client, 'show-cloud', (n, '--format', 'yaml'))
        for n in cloud_dict
        ], gjo_mock.mock_calls)


class TestGetClouds(TestCase):

    def get_clouds(self, cloud_dict):
        client = fake_juju_client()
        with override_clouds(client, cloud_dict):
            return get_clouds(client)

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


class TestAssessClouds(TestCase):

    def test_asssess_clouds_no_clouds(self):
        client = fake_juju_client()
        with override_clouds(client, {}):
            assess_clouds(client, {})

    def test_asssess_clouds_with_clouds(self):
        client = fake_juju_client()
        with override_clouds(client, {'localhost1': {
                'defined': 'local',
                'type': 'maas',
                'description': 'Metal As A Service',
                }}):
            assess_clouds(client, {'localhost1': {'type': 'maas'}})

    def test_asssess_clouds_mismatch(self):
        client = fake_juju_client()
        with override_clouds(client, {'localhost1': {
                'defined': 'local',
                'type': 'maas',
                'description': 'Metal As A Service',
                }}):
            with self.assertRaises(JujuAssertionError):
                assess_clouds(client, {})


class TestAssessShowCloud(TestCase):

    def test_asssess_show_cloud(self):
        client = fake_juju_client()
        with override_show_cloud(self, client, {
                'localhost1': {
                    'defined': 'local',
                    'type': 'maas',
                    'description': 'Metal As A Service',
                    },
                'localhost2': {
                    'defined': 'local',
                    'type': 'openstack',
                    'description': 'Openstack Cloud',
                    },
                }):
            assess_show_cloud(client, {
                'localhost1': {'type': 'maas'},
                'localhost2': {'type': 'openstack'},
                })

    def test_asssess_show_cloud_mismatch(self):
        client = fake_juju_client()
        with override_show_cloud(self, client, {
                'localhost1': {
                    'defined': 'local',
                    'type': 'maas',
                    'description': 'Metal As A Service',
                    },
                'localhost2': {
                    'defined': 'local',
                    'type': 'openstack',
                    'description': 'Openstack Cloud',
                    },
                }):
            with self.assertRaises(JujuAssertionError):
                assess_show_cloud(client, {
                    'localhost1': {'type': 'openstack'},
                    'localhost2': {'type': 'openstack'},
                    })
