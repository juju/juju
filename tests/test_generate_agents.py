import os
from unittest import TestCase

from generate_agents import (
    move_debs,
    NoDebsFound,
    )
from utils import temp_dir


class TestMoveDebs(TestCase):

    def test_juju2(self):
        with temp_dir() as dest_debs:
            parent = os.path.join(dest_debs, 'juju2')
            os.mkdir(parent)
            open(os.path.join(parent, 'foo.deb'), 'w').close()
            move_debs(dest_debs)
            self.assertTrue(os.path.exists(os.path.join(dest_debs, 'foo.deb')))

    def test_juju_core(self):
        with temp_dir() as dest_debs:
            parent = os.path.join(dest_debs, 'juju-core')
            os.mkdir(parent)
            open(os.path.join(parent, 'foo.deb'), 'w').close()
            move_debs(dest_debs)
            self.assertTrue(os.path.exists(os.path.join(dest_debs, 'foo.deb')))

    def test_none(self):
        with temp_dir() as dest_debs:
            parent = os.path.join(dest_debs, 'juju-core')
            os.mkdir(parent)
            with self.assertRaisesRegexp(NoDebsFound, 'No deb files found.'):
                move_debs(dest_debs)

    def test_wrong_dir(self):
        with temp_dir() as dest_debs:
            parent = os.path.join(dest_debs, 'wrong-dir')
            os.mkdir(parent)
            open(os.path.join(parent, 'foo.deb'), 'w').close()
            with self.assertRaisesRegexp(NoDebsFound, 'No deb files found.'):
                move_debs(dest_debs)
