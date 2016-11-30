from tests import TestCase

from assess_cloud_display import remove_display_attributes
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
