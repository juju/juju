"""Tests for assess_perf_test_simple module."""

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

from fakejuju import fake_juju_client
import generate_perfscale_results as gpr
import perf_graphing
from test_perfscale_deployment import get_default_args
from test_quickstart_deploy import make_bootstrap_manager
from tests import TestCase
from utility import temp_dir


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


class TestDumpPerformanceMetricsLogs(TestCase):

    def test_pulls_rrd_and_mongostats_logs(self):
        client = Mock()
        res_dir = '/foo/performance_results/'

        with patch.object(gpr.os, 'makedirs', autospec=True) as m_makedirs:
            self.assertEqual(
                res_dir,
                gpr.dump_performance_metrics_logs('/foo', client))
        m_makedirs.assert_called_once_with(res_dir)
        expected_calls = [
            call(
                'scp',
                ('--', '-r', '0:/var/lib/collectd/rrd/localhost/*', res_dir)),
            call(
                'scp',
                ('0:/tmp/mongodb-stats.log', res_dir)
                )]
        self.assertEqual(
            client.juju.call_args_list,
            expected_calls)


class TestJsonSerialisation(TestCase):

    def test_TimingData_serialises(self):
        timing_data = gpr.TimingData(datetime.utcnow(), datetime.utcnow())
        json.dumps(timing_data, cls=gpr.PerfTestDataJsonSerialisation)

    def test_DeployDetails_serialises(self):
        timing_data = gpr.TimingData(datetime.utcnow(), datetime.utcnow())
        deploy_details = gpr.DeployDetails(
            'name', dict(app_name=1), timing_data)
        json.dumps(deploy_details, cls=gpr.PerfTestDataJsonSerialisation)

    def test_serialise_stores_values(self):
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
        with patch.object(
                gpr, 'create_report_graph', return_value=image) as m_crg:
            self.assertEqual(
                image,
                gpr.generate_graph_image(
                    base_dir, results_dir, name, generator))
        m_crg.assert_called_once_with(
            '/foo/test/results', '/foo/test/testing_name.png', generator)

    def test_generate_cpu_graph(self):
        image = Mock()
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_cpu_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo', 'aggregation-cpu-average', 'cpu', perf_graphing.cpu_graph)

    def test_generate_memory_graph_calls_(self):
        image = Mock()
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_memory_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo', 'memory', 'memory', perf_graphing.memory_graph)

    def test_generate_network_graph(self):
        image = Mock()
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_network_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo', 'interface-eth0', 'network', perf_graphing.network_graph)

    def test_generate_mongo_query_graph(self):
        image = Mock()
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_mongo_query_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo', 'mongodb', 'mongodb', perf_graphing.mongodb_graph)

    def test_generate_mongo_memory_graph(self):
        image = Mock()
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_mongo_memory_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo',
            'mongodb',
            'mongodb_memory',
            perf_graphing.mongodb_memory_graph)

    def test_create_report_graph_returns_base_file_path(self):
        """The returned filepath should just be the basename."""
        generator = Mock()
        start = 0000
        end = 9999
        file_list = ['example.rrd']
        rrd_dir = '/foo'
        output_file = '/bar/test.png'
        output_file_base = 'test.png'

        with patch.object(
                gpr.os, 'listdir',
                autospec=True, return_value=file_list) as m_list:
            with patch.object(
                    gpr, 'get_duration_points',
                    autospec=True, return_value=(start, end)) as m_gdp:
                self.assertEqual(
                    output_file_base,
                    gpr.create_report_graph(rrd_dir, output_file, generator)
                )
        m_gdp.assert_called_once_with('/foo/example.rrd')
        m_list.assert_called_once_with(rrd_dir)
        generator.assert_called_once_with(start, end, rrd_dir, output_file)


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
