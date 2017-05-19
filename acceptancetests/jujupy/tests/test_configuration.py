import os
from unittest import TestCase

from mock import patch

from jujupy.configuration import (
    get_jenv_path,
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


class TestGetJenvPath(TestCase):

    def test_get_jenv_path(self):
        self.assertEqual('home/environments/envname.jenv',
                         get_jenv_path('home', 'envname'))
