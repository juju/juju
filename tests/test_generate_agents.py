import os
from unittest import TestCase

from generate_agents import (
    make_centos_agent,
    make_windows_agent,
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


class TestMakeCentosAgent(TestCase):

    def test_make_centos_agent(self):
        with temp_dir() as workspace:
            dest_debs = os.path.join(workspace, 'debs')
            agent_dir = os.path.join(dest_debs, 'agent', '1.25.5')
            os.makedirs(agent_dir)
            stanzas = os.path.join(workspace, 'stanzas')
            os.mkdir(stanzas)
            agent = os.path.join(dest_debs, 'juju-1.25.5-centos7-amd64.tgz')
            with open(agent, 'w') as dummy_file:
                dummy_file.write('centos7 agent')
            make_centos_agent(dest_debs, 'proposed', '1.25.5')
            agent_path = os.path.join(
                workspace, 'debs', 'agent', '1.25.5',
                'juju-1.25.5-centos7-amd64.tgz')
            self.assertTrue(os.path.exists(agent_path))
            stanza_path = os.path.join(
                workspace, 'debs', 'proposed-1.25.5-centos.json')
            self.assertTrue(os.path.exists(stanza_path))


class TestMakeWindowsAgent(TestCase):

    def test_make_windows_agent(self):
        with temp_dir() as workspace:
            dest_debs = os.path.join(workspace, 'debs')
            agent_dir = os.path.join(dest_debs, 'agent', '1.25.5')
            os.makedirs(agent_dir)
            stanzas = os.path.join(workspace, 'stanzas')
            os.mkdir(stanzas)
            agent = os.path.join(dest_debs, 'juju-1.25.5-win2012-amd64.tgz')
            with open(agent, 'w') as dummy_file:
                dummy_file.write('windows agent')
            make_windows_agent(dest_debs, 'proposed', '1.25.5')
            agent_path = os.path.join(
                workspace, 'debs', 'agent', '1.25.5',
                'juju-1.25.5-windows-amd64.tgz')
            self.assertTrue(os.path.exists(agent_path))
            stanza_path = os.path.join(
                workspace, 'debs', 'proposed-1.25.5-windows.json')
            self.assertTrue(os.path.exists(stanza_path))
