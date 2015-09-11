from mock import patch
from unittest import TestCase

from download_juju import (
    select_build
)
class SelectBuild(TestCase):

    def test_select_build_(self):
        builds = [
            'j/p/r/build-win-client/build-829/c/juju-setup-1.25-alpha1.exe',
            ]
        build = select_build(builds)
        self.assertEqual(
            build, 's3://juju-qa-data/j/p/r/build-win-client/build-829')

    def test_select_build(self):
        builds = [
            'j/p/r/build-win-client/build-829/c/juju-setup-1.25-alpha1.exe',
            'j/p/r/build-win-client/build-835/c/juju-setup-1.25-alpha1.exe',
            'j/p/r/build-win-client/build-1000/c/juju-setup-1.25-alpha1.exe',
            'j/p/r/build-win-client/build-10/c/juju-setup-1.25-alpha1.exe',
            'j/p/v/build-win-client/build-2000/c/juju-setup-1.25-alpha1.exe',
            'j/p/r/build-win-client/build-900/c/juju-setup-1.25-alpha1.exe',
        ]
        build = select_build(builds)
        self.assertEqual(
            build, 's3://juju-qa-data/j/p/v/build-win-client/build-2000')
