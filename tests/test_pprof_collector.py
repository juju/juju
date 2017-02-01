"""Tests for assess_perf_test_simple module."""

from contextlib import contextmanager
from datetime import datetime
import os
from mock import call, patch, Mock, mock_open

import pprof_collector as pc
from tests import TestCase


class TestActiveCollector(TestCase):

    def test_installs_introspection_charm(self):
        client = Mock()
        with patch.object(
                pc, 'install_introspection_charm', autospec=True) as iic:
            pc.ActiveCollector(client, 'test')
        iic.assert_called_once_with(client, 'test')

    def test__collect_profile_uses_correct_profile_url(self):
        client = Mock()
        expected_url = pc.get_profile_url(
            '123.123.123.123', 'test', 'test_profile', '12')
        with patch.object(
                pc, 'install_introspection_charm',
                return_value='123.123.123.123',
                autospec=True):
            with patch.object(pc, 'get_profile_reading', autospec=True) as gpr:
                collector = pc.ActiveCollector(client, 'test')
                collector._collect_profile('test_profile', '/tmp/test', '12')
        gpr.assert_called_once_with(expected_url, '/tmp/test')

    @contextmanager
    def mocked_collector(self, machine_id='test'):
        """Create an ActiveCollector with patched _collect_profile.

        Used to test collect_* methods.
        """
        client = Mock()
        with patch.object(
                pc, 'install_introspection_charm',
                return_value='123.123.123.123',
                autospec=True):
            collector = pc.ActiveCollector(client, machine_id)
            with patch.object(collector, '_collect_profile'):
                yield collector

    def test_collect_profile(self):
        with self.mocked_collector() as mc:
            mc.collect_profile('/path/profile', 42)
            mc._collect_profile.assert_called_once_with(
                'profile', '/path/profile', 42)

    def test_collect_heap(self):
        with self.mocked_collector() as mc:
            mc.collect_heap('/path/heap', 42)
            mc._collect_profile.assert_called_once_with(
                'heap', '/path/heap', 42)

    def test_collect_goroutines(self):
        with self.mocked_collector() as mc:
            mc.collect_goroutines('/path/goroutines', 42)
            mc._collect_profile.assert_called_once_with(
                'goroutines', '/path/goroutines', 42)


class TestHelperFunctions(TestCase):

    def test_get_profile_url(self):
        url = pc.get_profile_url('1.1.1.1', 'test', 'test_profile', 123)
        self.assertEqual(
            url,
            'http://1.1.1.1:19090/agents/machine-test/debug/pprof/test_profile'
            '?seconds=123'
        )

    def test_get_profile_reading(self):
        m = mock_open()
        open_patch = '{}.open'.format(pc.__name__)
        mock_response = Mock()
        mock_response.content = 'This is some content.'
        with patch.object(pc, 'requests', autospect=True) as pr:
            pr.get.return_value = mock_response
            with patch(open_patch, m, create=True):
                pc.get_profile_reading('test/url', '/tmp/testing/url')
                m.assert_called_once_with('/tmp/testing/url', 'wb')
                handle = m()
                handle.write.assert_called_once_with('This is some content.')

    def test_installs_introspection_charm(self):
        client = Mock()
        with patch.object(pc, 'get_unit_ipaddress', autospec=True) as p_pui:
            pc.install_introspection_charm(client, 'test')

        client.deploy.assert_called_once_with(
            'cs:~axwalk/juju-introspection', to='test')
        client.wait_for_started.assert_called_once_with()
        client.wait_for_workloads.assert_called_once_with()
        client.juju.assert_called_once_with('expose', 'juju-introspection')
        p_pui.assert_called_once_with(client, 'juju-introspection/0')


class TestPPROFCollector(TestCase):

    def test_creates_profile_directories(self):
        client = Mock()
        log_dir = '/test/logs/dir'
        with patch.object(pc.os, 'makedirs', autospec=True) as p_md:
            pc.PPROFCollector(client, ['test'], log_dir)

        self.assertListEqual(
            p_md.call_args_list,
            [
                call(os.path.join(log_dir, 'cpu_profile')),
                call(os.path.join(log_dir, 'heap_profile')),
                call(os.path.join(log_dir, 'goroutines_profile'))
            ]
        )

    def test_defaults_to_non_active_collector(self):
        client = Mock()
        log_dir = '/test/logs/dir'
        with patch.object(pc.os, 'makedirs', autospec=True):
            collector = pc.PPROFCollector(client, ['test'], log_dir)

            self.assertTrue(type(collector._collectors[0]), pc.NoopCollector)
            self.assertListEqual(collector._active_collectors, [])

    def test_creates_active_collector(self):
        client = Mock()
        log_dir = '/test/logs/dir'
        with patch.object(pc.os, 'makedirs', autospec=True):
            with patch.object(
                    pc, 'install_introspection_charm',
                    autospec=True):
                collector = pc.PPROFCollector(
                    client, ['test'], log_dir, active=True)
                self.assertTrue(
                    type(collector._collectors[0]),
                    pc.ActiveCollector)
                self.assertListEqual(collector._noop_collectors, [])

    def test_initiates_active_collector_after_creation(self):
        client = Mock()
        log_dir = '/test/logs/dir'
        with patch.object(pc.os, 'makedirs', autospec=True):
            collector = pc.PPROFCollector(client, ['test'], log_dir)
            self.assertTrue(type(collector._collectors[0]), pc.NoopCollector)
            with patch.object(
                    pc, 'install_introspection_charm',
                    autospec=True):
                self.assertListEqual(collector._active_collectors, [])
                collector.set_active()
                self.assertTrue(
                    type(collector._collectors[0]), pc.NoopCollector)

    def test_collect_profile(self):
        client = Mock()
        log_dir = '/test/logs/dir'
        file_name = 'machine-42-170130-092626.pprof'
        profile_log = os.path.join(log_dir, 'cpu_profile', file_name)
        with patch.object(pc.os, 'makedirs', autospec=True):
            with patch.object(pc, 'NoopCollector', autospec=True) as m_nc:
                mock_noop = Mock()
                mock_noop.machine_id = 42
                m_nc.return_value = mock_noop
                with patch.object(pc, 'datetime') as p_dt:
                    p_dt.utcnow.return_value = datetime(
                        2017, 1, 30, 9, 26, 26, 587930)
                    collector = pc.PPROFCollector(client, ['test'], log_dir)
                    collector.collect_profile()
        collector._collectors[0].collect_profile.assert_called_once_with(
            profile_log, 5
        )

    def test_collect_heap(selfm):
        client = Mock()
        log_dir = '/test/logs/dir'
        file_name = 'machine-42-170130-092626.pprof'
        profile_log = os.path.join(log_dir, 'heap_profile', file_name)
        with patch.object(pc.os, 'makedirs', autospec=True):
            with patch.object(pc, 'NoopCollector', autospec=True) as m_nc:
                mock_noop = Mock()
                mock_noop.machine_id = 42
                m_nc.return_value = mock_noop
                with patch.object(pc, 'datetime') as p_dt:
                    p_dt.utcnow.return_value = datetime(
                        2017, 1, 30, 9, 26, 26, 587930)
                    collector = pc.PPROFCollector(client, ['test'], log_dir)
                    collector.collect_heap()
        collector._collectors[0].collect_heap.assert_called_once_with(
            profile_log, 5
        )

    def test_collect_goroutines(selfm):
        client = Mock()
        log_dir = '/test/logs/dir'
        file_name = 'machine-42-170130-092626.pprof'
        profile_log = os.path.join(log_dir, 'goroutines_profile', file_name)
        with patch.object(pc.os, 'makedirs', autospec=True):
            with patch.object(pc, 'NoopCollector', autospec=True) as m_nc:
                mock_noop = Mock()
                mock_noop.machine_id = 42
                m_nc.return_value = mock_noop
                with patch.object(pc, 'datetime') as p_dt:
                    p_dt.utcnow.return_value = datetime(
                        2017, 1, 30, 9, 26, 26, 587930)
                    collector = pc.PPROFCollector(client, ['test'], log_dir)
                    collector.collect_goroutines()
        collector._collectors[0].collect_goroutines.assert_called_once_with(
            profile_log, 5
        )
