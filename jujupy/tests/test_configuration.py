from unittest import TestCase

from jujupy.configuration import (
    get_jenv_path,
)


class TestGetJenvPath(TestCase):

    def test_get_jenv_path(self):
        self.assertEqual('home/environments/envname.jenv',
                         get_jenv_path('home', 'envname'))
