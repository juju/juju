import os
from unittest import TestCase

try:
    from mock import patch
except ImportError:
    from unittest.mock import patch

from jujupy.configuration import (
    get_juju_data,
)


class TestGetJujuData(TestCase):

    def test_from_home(self):
        with patch.dict(os.environ, {
                        'HOME': 'foo-bar',
                        }, clear=True):
            self.assertEqual(get_juju_data(), 'foo-bar/.local/share/juju')

    def test_from_data_home(self):
        with patch.dict(os.environ, {
                        'HOME': 'foo-bar',
                        'XDG_DATA_HOME': 'baz',
                        }, clear=True):
            self.assertEqual(get_juju_data(), 'baz/juju')

    def test_from_juju_data(self):
        with patch.dict(os.environ, {
                        'HOME': 'foo-bar',
                        'XDG_DATA_HOME': 'baz',
                        'JUJU_DATA': 'qux',
                        }, clear=True):
            self.assertEqual(get_juju_data(), 'qux')
