"""Tests for assess_perf_test_simple module."""

import argparse
from contextlib import contextmanager
from collections import OrderedDict
from datetime import (
    datetime,
    timedelta,
)
import os
from mock import call, patch, Mock
from textwrap import dedent
import json

from jujupy import fake_juju_client
import generate_perfscale_results as gpr
import perf_graphing
from test_quickstart_deploy import make_bootstrap_manager
from tests import TestCase
from utility import temp_dir


def get_default_args(**kwargs):
    default_args = dict(
        env='an-env',
        juju_bin='/bin/juju',
        logs='/tmp/logs',
        temp_env_name='an-env-mod',
        enable_ha=False,
        enable_pprof=False,
        debug=False,
        agent_stream=None,
        agent_url=None,
        bootstrap_host=None,
        keep_env=False,
        machine=[],
        region=None,
        series=None,
        upload_tools=False,
        verbose=20,
        deadline=None)
    default_args.update(kwargs)

    return argparse.Namespace(**default_args)


class TestAddBasicPerfscaleArguments(TestCase):

    def test_adds_perfscale_arguments(self):
        parser = argparse.ArgumentParser()
        gpr.add_basic_perfscale_arguments(parser)
        parsed_args = parser.parse_args([])
        self.assertEqual(parsed_args.enable_ha, False)
        self.assertEqual(parsed_args.enable_pprof, False)

        parsed_args = parser.parse_args(['--enable-ha', '--enable-pprof'])
        self.assertEqual(parsed_args.enable_ha, True)
        self.assertEqual(parsed_args.enable_pprof, True)

    def test_includes_basic_default_arguments(self):
        parser = argparse.ArgumentParser()
        gpr.add_basic_perfscale_arguments(parser)
        parsed_args = parser.parse_args([
            'an-env',
            '/bin/juju',
            '/tmp/logs',
            'an-env-mod'])

        self.assertEqual(parsed_args, get_default_args())


class TestMaybeEnableHA(TestCase):

    def test_must_not_be_enabled_by_default(self):
        parser = argparse.ArgumentParser()
        gpr.add_basic_perfscale_arguments(parser)
        args = parser.parse_args([])

        client = Mock()
        gpr.maybe_enable_ha(client, args)

        self.assertEqual(0, client.enable_ha.call_count)
        self.assertEqual(0, client.wait_for_ha.call_count)

    def test_must_be_enabled_when_requested(self):
        parser = argparse.ArgumentParser()
        gpr.add_basic_perfscale_arguments(parser)
        args = parser.parse_args(['--enable-ha'])

        client = Mock()
        gpr.maybe_enable_ha(client, args)

        client.enable_ha.assert_called_once_with()
        client.wait_for_ha.assert_called_once_with()


class TestRunPerfscaleTest(TestCase):

    def test_calls_provided_test(self):
        client = fake_juju_client()
        with temp_dir() as juju_home:
            client.env.juju_home = juju_home
            bs_manager = make_bootstrap_manager(client)
            bs_manager.log_dir = os.path.join(juju_home, 'log-dir')
            os.mkdir(bs_manager.log_dir)

            timing = gpr.TimingData(datetime.utcnow(), datetime.utcnow())
            deploy_details = gpr.DeployDetails('test', dict(), timing)
            noop_test = Mock(return_value=deploy_details)

            with patch.object(gpr, 'dump_performance_metrics_logs',
                              autospec=True):
                with patch.object(gpr, 'generate_reports', autospec=True):
                    gpr.run_perfscale_test(noop_test, bs_manager,
                                           get_default_args())

            noop_test.assert_called_once_with(client, get_default_args())


class TestGetControllerMachines(TestCase):

    def test_returns_machine_id_for_non_ha(self):
        client = fake_juju_client()
        client.bootstrap()
        self.assertListEqual(
            ['0'],
            gpr.get_controller_machine_ids(client.get_controller_client()))

    def test_returns_machine_id_for_ha_enabled(self):
        client = fake_juju_client()
        client.bootstrap()
        client.enable_ha()
        self.assertListEqual(
            ['0', '1', '2'],
            gpr.get_controller_machine_ids(client.get_controller_client()))


class TestSetupSystemMonitoring(TestCase):

    def test_setup_for_non_ha(self):
        client = fake_juju_client()
        client.bootstrap()
        admin_client = client.get_controller_client()

        with patch.object(
                gpr, '_setup_system_monitoring', autospec=True) as m_ssm:
            with patch.object(
                    gpr, '_enable_monitoring', autospec=True) as m_em:
                self.assertListEqual(
                    ['0'],
                    gpr.setup_system_monitoring(admin_client))
        m_ssm.assert_called_once_with(admin_client, '0')
        m_em.assert_called_once_with(admin_client, '0')

    def test_setup_for_ha_enabled(self):
        client = fake_juju_client()
        client.bootstrap()
        client.enable_ha()
        admin_client = client.get_controller_client()

        with patch.object(
                gpr, '_setup_system_monitoring', autospec=True) as m_ssm:
            with patch.object(
                    gpr, '_enable_monitoring', autospec=True) as m_em:
                self.assertListEqual(
                    ['0', '1', '2'],
                    gpr.setup_system_monitoring(admin_client))
        self.assertListEqual(
            m_ssm.call_args_list,
            [
                call(admin_client, '0'),
                call(admin_client, '1'),
                call(admin_client, '2')])
        self.assertListEqual(
            m_em.call_args_list,
            [
                call(admin_client, '0'),
                call(admin_client, '1'),
                call(admin_client, '2')])

    def test__system_monitoring_installs_on_requested_machine(self):
        admin_client = Mock()
        static_setup_path = '/foo/bar/setup'
        static_config_path = '/baz/bang/config'

        expected_calls = [
            call(
                'scp',
                ('--proxy', static_config_path, '2:/tmp/collectd.config')),
            call('scp', ('--proxy', static_setup_path, '2:/tmp/installer.sh')),
            call('ssh', ('--proxy', '2', 'chmod +x /tmp/installer.sh')),
        ]

        with patch.object(gpr, 'PATHS', autospec=True) as m_sp:
            m_sp.installer_script_path = static_setup_path
            m_sp.collectd_config_path = static_config_path
            m_sp.collectd_config_dest_file = '/tmp/collectd.config'
            m_sp.installer_script_dest_path = '/tmp/installer.sh'
            gpr._setup_system_monitoring(admin_client, '2')

        self.assertListEqual(
            admin_client.juju.call_args_list,
            expected_calls)

    def test__enable_monitoring_enables_script_on_controller_machine(self):
        admin_client = Mock()
        gpr._enable_monitoring(admin_client, '3')
        expected_calls = [
            call(
                'ssh',
                ('--proxy',
                 '3',
                 '/tmp/installer.sh /tmp/collectd.config /tmp/runner.sh')),
            call(
                'ssh',
                ('--proxy', '3', '--', 'daemon --respawn /tmp/runner.sh'))]
        self.assertListEqual(
            admin_client.juju.call_args_list,
            expected_calls
        )


class TestDumpPerformanceMetricsLogs(TestCase):

    def test_pulls_rrd_and_mongostats_logs_with_single_controller(self):
        client = Mock()
        res_dir = '/foo/performance_results/'
        expected_dir = '{}machine-0'.format(res_dir)
        machine_ids = ['0']

        with patch.object(gpr.os, 'makedirs', autospec=True) as m_makedirs:
            self.assertEqual(
                res_dir,
                gpr.dump_performance_metrics_logs('/foo', client, machine_ids))
        m_makedirs.assert_called_once_with(expected_dir)
        expected_calls = [
            call(
                'scp',
                ('--proxy', '--', '-r',
                 '0:/var/lib/collectd/rrd/localhost/*', expected_dir)),
            call(
                'scp',
                ('--proxy', '0:/tmp/mongodb-stats.log', expected_dir)
                )]
        self.assertEqual(
            client.juju.call_args_list,
            expected_calls)

    def test_pulls_rrd_and_mongostats_logs_when_ha_enabled(self):
        client = Mock()
        res_dir = '/foo/performance_results/'
        machine_ids = ['0', '1', '2']

        with patch.object(gpr.os, 'makedirs', autospec=True) as m_makedirs:
            self.assertEqual(
                res_dir,
                gpr.dump_performance_metrics_logs('/foo', client, machine_ids))

        makedir_calls = [
            call('/foo/performance_results/machine-0'),
            call('/foo/performance_results/machine-1'),
            call('/foo/performance_results/machine-2')]
        self.assertListEqual(
            m_makedirs.call_args_list,
            makedir_calls)

        expected_calls = []
        for m_id in machine_ids:
            expected_dir = '{}machine-{}'.format(res_dir, m_id)
            expected_calls.append(
                call(
                    'scp',
                    ('--proxy', '--', '-r',
                     '{}:/var/lib/collectd/rrd/localhost/*'.format(m_id),
                     expected_dir)))
            expected_calls.append(
                call(
                    'scp',
                    ('--proxy',
                     '{}:/tmp/mongodb-stats.log'.format(m_id), expected_dir)))

        self.assertEqual(
            client.juju.call_args_list,
            expected_calls)


class TestGetControllerLogMessageChunks(TestCase):

    def test_retains_backwards_compatiable_naming(self):
        # Previous to the HA addtions the stored data was expected to have a
        # single machines log under the name 'log_message_chunks'.
        log_dir = '/foo/logs'
        machine_ids = ['0']
        deployments = dict(
            bootstrap='bootstrap', cleanup='cleanup', deploys='deploys')
        graph_period = '0'

        with patch.object(
                gpr, 'breakdown_log_by_events_timeframe',
                autospec=True) as m_blbet:
            results = gpr._get_controller_log_message_chunks(
                log_dir, machine_ids, deployments, graph_period)

        self.assertListEqual(
            results.keys(),
            ['log_message_chunks']
        )

        source_log = '/foo/logs/controller/machine-0/machine-0.log.gz'
        m_blbet.assert_called_once_with(
            source_log, 'bootstrap', 'cleanup', 'deploys')

    def test_creates_log_chunks_for_each_controller(self):
        # Must chunk logs for each controller.
        log_dir = '/foo/logs'
        machine_ids = ['0', '1', '2']
        deployments = dict(
            bootstrap='bootstrap', cleanup='cleanup', deploys='deploys')
        graph_period = '0'

        with patch.object(
                gpr, 'breakdown_log_by_events_timeframe',
                autospec=True) as m_blbet:
            results = gpr._get_controller_log_message_chunks(
                log_dir, machine_ids, deployments, graph_period)

        self.assertListEqual(
            sorted(results.keys()),
            ['log_message_chunks',
             'log_message_chunks_1',
             'log_message_chunks_2']
        )

        expected_calls = [
            call('/foo/logs/controller/machine-0/machine-0.log.gz',
                 'bootstrap', 'cleanup', 'deploys'),
            call('/foo/logs/controller/machine-1/machine-1.log.gz',
                 'bootstrap', 'cleanup', 'deploys'),
            call('/foo/logs/controller/machine-2/machine-2.log.gz',
                 'bootstrap', 'cleanup', 'deploys')]
        self.assertListEqual(
            m_blbet.call_args_list,
            expected_calls)


class TestJsonSerialisation(TestCase):

    def test_serialise_stores_values(self):
        """Must serialise data for TimingData and DeployDetails objects."""
        start = datetime.utcnow()
        end = datetime.utcnow()
        seconds = int((end-start).total_seconds())
        app_details = dict(app_name=1)
        timing_data = gpr.TimingData(start, end)
        deploy_details = gpr.DeployDetails(
            'name', app_details, timing_data)
        json_string = json.dumps(
            deploy_details, cls=gpr.PerfTestDataJsonSerialisation)

        stored_data = json.loads(json_string)

        self.assertEqual(stored_data['name'], 'name')
        self.assertEqual(stored_data['applications'], app_details)
        self.assertEqual(stored_data['timings']['start'], timing_data.start)
        self.assertEqual(stored_data['timings']['end'], timing_data.end)
        self.assertEqual(stored_data['timings']['seconds'], seconds)


class TestGenerateGraphImages(TestCase):

    @contextmanager
    def patch_image_creation_ctx(self, image):
        with patch.object(
                gpr, 'generate_graph_image',
                return_value=image, autospec=True) as m_ggi:
            yield m_ggi

    def test_generate_graph_image(self):
        image = Mock()
        base_dir = '/foo/test'
        results_dir = 'results'
        name = 'testing_name'
        generator = Mock()
        graph_period = '0'
        with patch.object(
                gpr, 'create_report_graph', return_value=image) as m_crg:
            self.assertEqual(
                image,
                gpr.generate_graph_image(
                    base_dir, results_dir, name, generator, graph_period))
        m_crg.assert_called_once_with(
            '/foo/test/results',
            '/foo/test/test-testing_name.png',
            generator,
            graph_period)

    def test_generate_cpu_graph(self):
        image = Mock()
        graph_period = '0'
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_cpu_graph_image('/foo', graph_period))
        m_ggi.assert_called_once_with(
            '/foo',
            'aggregation-cpu-max',
            'cpu',
            perf_graphing.cpu_graph,
            graph_period)

    def test_generate_memory_graph_calls_(self):
        image = Mock()
        graph_period = '0'
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_memory_graph_image('/foo', graph_period))
        m_ggi.assert_called_once_with(
            '/foo',
            'memory',
            'memory',
            perf_graphing.memory_graph,
            graph_period)

    def test_generate_network_graph(self):
        image = Mock()
        graph_period = '0'
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_network_graph_image('/foo', graph_period))
        m_ggi.assert_called_once_with(
            '/foo',
            'interface-eth0',
            'network',
            perf_graphing.network_graph,
            graph_period)

    def test_generate_mongo_query_graph(self):
        image = Mock()
        graph_period = '0'
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_mongo_query_graph_image('/foo', graph_period))
        m_ggi.assert_called_once_with(
            '/foo',
            'mongodb',
            'mongodb',
            perf_graphing.mongodb_graph,
            graph_period)

    def test_generate_mongo_memory_graph(self):
        image = Mock()
        graph_period = '0'
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_mongo_memory_graph_image('/foo', graph_period))
        m_ggi.assert_called_once_with(
            '/foo',
            'mongodb',
            'mongodb_memory',
            perf_graphing.mongodb_memory_graph,
            graph_period)

    def test_create_report_graph_returns_base_file_path(self):
        """The returned filepath should just be the basename."""
        generator = Mock()
        start = 0000
        end = 9999
        file_list = ['example.rrd']
        rrd_dir = '/foo'
        output_file = '/bar/test.png'
        output_file_base = 'test.png'
        graph_period = '0'

        with patch.object(
                gpr.os, 'listdir',
                autospec=True, return_value=file_list) as m_list:
            with patch.object(
                    gpr, 'get_duration_points',
                    autospec=True, return_value=(start, end)) as m_gdp:
                self.assertEqual(
                    output_file_base,
                    gpr.create_report_graph(
                        rrd_dir, output_file, generator, graph_period)
                )
        m_gdp.assert_called_once_with('/foo/example.rrd', graph_period)
        m_list.assert_called_once_with(rrd_dir)
        generator.assert_called_once_with(
            start, end, rrd_dir, output_file)


class TestFindActualStart(TestCase):
    example_output = dedent("""\
                         value

    1468551204: -nan
    1468554270: -nan
    1468554273: -nan
    1468554270: -nan
    1468554273: -nan
    1468554276: 1.7516817067e+08
    1468554279: 1.7500023467e+08
    1468554282: 1.7661269333e+08
    1468554285: 1.7819374933e+08""")

    example_multivalue_output = dedent("""\
                             value1    value2

    1472708601: -nan -nan
    1472708604: -nan -nan
    1472708607: -nan -nan
    1472708610: -nan -nan
    1472708613: 7.5466666667e+02 5.8166666667e+02
    1472708616: 2.5555555556e+02 1.9833333333e+02
    1472708619: 1.3333333333e+01 1.1555555556e+01
    1472708622: 2.7444444444e+01 2.6222222222e+01""")

    def test_doesnt_choke_on_non_timestamp_lines(self):
        try:
            gpr.find_actual_start(self.example_output)
            gpr.find_actual_start(self.example_multivalue_output)
        except Exception:
            self.fail('Unexpected exception raised.')

    def test_returns_actual_start_timestamp(self):
        self.assertEqual(
            gpr.find_actual_start(self.example_output),
            '1468554276')

        self.assertEqual(
            gpr.find_actual_start(self.example_multivalue_output),
            '1472708613')


class TestLogBreakdown(TestCase):

    def test__get_chunked_log(self):
        log_file = '/path/to/file'
        deployments = ['deploy1', 'deploy2']
        with patch.object(
                gpr,
                'breakdown_log_by_timeframes',
                return_value='chunked_log',
                autospec=True) as m_blt:
            self.assertEqual(
                'chunked_log',
                gpr._get_chunked_log(
                    log_file, 'bootstrap', 'cleanup', deployments))
        m_blt.assert_called_once_with(
            log_file, ['bootstrap'] + deployments + ['cleanup'])

    def test__get_log_name_lookup_table(self):
        """Must return a dict of the date range as key and a display name."""
        start = datetime.utcnow()
        end = datetime.utcnow()
        bootstrap = gpr.TimingData(start, end)
        cleanup = gpr.TimingData(
            start + timedelta(seconds=1),
            end + timedelta(seconds=1))
        deploy_timing = gpr.TimingData(
            start + timedelta(seconds=2),
            end + timedelta(seconds=2))
        deployments = [
            gpr.DeployDetails('Deploy Name', {}, deploy_timing)]

        bs_name = gpr._render_ds_string(bootstrap.start, bootstrap.end)
        cleanup_name = gpr._render_ds_string(cleanup.start, cleanup.end)
        deploy_name = gpr._render_ds_string(
            deploy_timing.start, deploy_timing.end)

        table = gpr._get_log_name_lookup_table(bootstrap, cleanup, deployments)
        self.assertDictEqual(
            table,
            {
                bs_name: 'Bootstrap',
                cleanup_name: 'Kill-Controller',
                deploy_name: 'Deploy Name',
            }
        )

    def test__get_display_safe_daterange(self):
        self.assertEqual(
            '2016-10-16202806-2016-10-16202944',
            gpr._display_safe_daterange(
                '2016-10-16 20:28:06 - 2016-10-16 20:29:44')
        )

    def test__get_display_safe_timerange(self):
        self.assertEqual(
            '203013-203033',
            gpr._display_safe_timerange('20:30:13 - 20:30:33')
        )
        self.assertEqual(
            '203013-203033condensed',
            gpr._display_safe_timerange('20:30:13 - 20:30:33 (condensed)')
        )


class TestBreakdownLogByEventsTimeframe(TestCase):

    def test_returns_ordered_dictionary_of_details(self):
        """Must be ordered on the event_range."""
        # Use ordered dict here so we can check the returned order has changed
        # later on.
        first = '2016-10-16 20:28:06 - 2016-10-16 20:29:44'
        second = '2016-10-16 20:30:13 - 2016-10-16 20:32:21'
        fake_data = OrderedDict()
        fake_data[second] = {'20:30:13 - 20:30:33': 'Log message second'}
        fake_data[first] = {'20:28:06 - 20:28:26': 'Log message'}

        name_lookup = {first: 'First', second: 'Second'}

        with patch.object(
                gpr, '_get_chunked_log',
                return_value=fake_data, autospec=True):
            with patch.object(
                    gpr, '_get_log_name_lookup_table',
                    return_value=name_lookup, autospec=True):
                details = gpr.breakdown_log_by_events_timeframe(
                    '/tmp', 'boostrap', 'cleanup', [])

        self.assertIsInstance(details, OrderedDict)
        items = details.items()
        self.assertEqual(items[0][0], first)
        self.assertEqual(items[1][0], second)

    def test_contains_display_friendly_datestamp(self):
        first = '2016-10-16 20:28:06 - 2016-10-16 20:29:44'
        display = '2016-10-16202806-2016-10-16202944'
        fake_data = dict()
        fake_data[first] = {'20:28:06 - 20:28:26': 'Log message'}

        name_lookup = {first: 'First'}

        with patch.object(
                gpr, '_get_chunked_log',
                return_value=fake_data, autospec=True):
            with patch.object(
                    gpr, '_get_log_name_lookup_table',
                    return_value=name_lookup, autospec=True):
                details = gpr.breakdown_log_by_events_timeframe(
                    '/tmp', 'boostrap', 'cleanup', [])
        self.assertEqual(details[first]['event_range_display'], display)

    def test_contains_display_friendly_timestamp(self):
        first = '2016-10-16 20:28:06 - 2016-10-16 20:29:44'
        display_time = '202806-202826'
        fake_data = dict()
        fake_data[first] = {'20:28:06 - 20:28:26': 'Log message'}

        name_lookup = {first: 'First'}

        with patch.object(
                gpr, '_get_chunked_log',
                return_value=fake_data, autospec=True):
            with patch.object(
                    gpr, '_get_log_name_lookup_table',
                    return_value=name_lookup, autospec=True):
                details = gpr.breakdown_log_by_events_timeframe(
                    '/tmp', 'boostrap', 'cleanup', [])
        self.assertEqual(
            details[first]['logs'][0]['display_timeframe'],
            display_time)
