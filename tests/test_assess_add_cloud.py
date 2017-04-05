from contextlib import contextmanager
from copy import deepcopy
from StringIO import StringIO
from textwrap import dedent
from unittest import TestCase

from mock import (
    call,
    patch,
    )

from jujupy import (
    AuthNotAccepted,
    fake_juju_client,
    InvalidEndpoint,
    NameNotAccepted,
    TypeNotAccepted,
    )
from assess_add_cloud import (
    assess_all_clouds,
    assess_cloud,
    CloudMismatch,
    CloudSpec,
    CloudValidation,
    cloud_spec,
    EXCEEDED_LIMIT,
    iter_clouds,
    NameMismatch,
    write_status,
    xfail,
    )
from tests import FakeHomeTestCase
from utility import JujuAssertionError


class TestCloudSpec(TestCase):

    def test_cloud_spec(self):
        self.assertEqual(
            CloudSpec('label1', 'name1', {'config': '1'}, None, None),
            cloud_spec('label1', 'name1', {'config': '1'}))


class TestXFail(TestCase):

    def test_xfail(self):
        spec = CloudSpec('label', 'name', {'config': 'value'}, 'foo', 'bar')
        fail_spec = xfail(spec, 'baz', 'qux')
        self.assertEqual(fail_spec, CloudSpec(
            'label', 'name', {'config': 'value'}, 'qux', 'baz'))


class TestAssessCloud(FakeHomeTestCase):

    @contextmanager
    def cloud_client(self, clouds):
        client = fake_juju_client(juju_home=self.juju_home)
        client.env.load_yaml()

        def dump(cloud_name, cloud):
            client.env.write_clouds(client.env.juju_home,
                                    clouds)

        with patch.object(client, 'add_cloud_interactive', dump):
            yield client

    def test_assess_cloud(self):
        expected_cloud = {'clouds': {
            'foo': {
                'type': 'maas',
                'endpoint': 'http://bar.example.com',
                }}}
        with self.cloud_client(expected_cloud) as client:
            assess_cloud(client, 'foo', expected_cloud['clouds']['foo'])

    def test_assess_cloud_missing(self):
        with self.cloud_client({'clouds': {}}) as client:
            with self.assertRaisesRegexp(JujuAssertionError,
                                         'Clouds missing!'):
                assess_cloud(client, 'foo', {
                    'type': 'maas',
                    'endpoint': 'http://bar.example.com',
                    })

    def test_assess_cloud_mismatch(self):
        with self.cloud_client({'clouds': {'foo': {}}}) as client:
            with self.assertRaisesRegexp(JujuAssertionError,
                                         'Cloud mismatch'):
                stderr = StringIO()
                with patch('sys.stderr', stderr):
                    assess_cloud(client, 'foo', {
                        'type': 'maas',
                        'endpoint': 'http://bar.example.com',
                        })
        self.assertEqual(dedent("""
            Expected:
            {endpoint: 'http://bar.example.com', type: maas}

            Actual:
            {}
            """), stderr.getvalue())


class TestCloudValidation(FakeHomeTestCase):

    def test_2_0(self):
        validation = CloudValidation('2.0.0')
        self.assertIs('2.0.0', validation.version)
        self.assertIs(validation.NONE, validation.support)
        self.assertIsFalse(validation.is_basic)
        self.assertIsFalse(validation.is_endpoint)
        self.assertFalse(validation.has_endpoint('openstack'))
        self.assertIs(
            validation.NONE, CloudValidation('2.0-beta1').support)
        self.assertIs(
            validation.NONE, CloudValidation('2.0.3').support)

    def test_2_1(self):
        validation = CloudValidation('2.1.0')
        self.assertIs('2.1.0', validation.version)
        self.assertIs(validation.BASIC, validation.support)
        self.assertIsTrue(validation.is_basic)
        self.assertIsFalse(validation.is_endpoint)
        self.assertFalse(validation.has_endpoint('openstack'))
        self.assertIs(
            validation.BASIC, CloudValidation('2.1-beta1').support)
        self.assertIs(
            validation.BASIC, CloudValidation('2.1.3').support)

    def test_2_2_plus(self):
        validation = CloudValidation('2.2.0')
        self.assertIs('2.2.0', validation.version)
        self.assertIs(validation.ENDPOINT, validation.support)
        self.assertIsFalse(validation.is_basic)
        self.assertIsTrue(validation.is_endpoint)
        self.assertTrue(validation.has_endpoint('openstack'))
        self.assertFalse(validation.has_endpoint('manual'))
        self.assertIs(
            validation.ENDPOINT, CloudValidation('2.2-beta1').support)
        self.assertIs(
            validation.ENDPOINT, CloudValidation('2.2.1').support)
        self.assertIs(
            validation.ENDPOINT, CloudValidation('2.3-beta1').support)


long_text = 'A' * EXCEEDED_LIMIT
endpoint_validation = CloudValidation('2.2.0')
basic_validation = CloudValidation('2.1.0')


def make_long_endpoint(spec, validation, regions=False):
    config = deepcopy(spec.config)
    config['endpoint'] = long_text
    if regions:
        for region in config['regions'].values():
            region['endpoint'] = long_text
    spec = cloud_spec('long-endpoint-{}'.format(spec.name), spec.name, config,
                      InvalidEndpoint)
    if validation.is_basic:
        spec = xfail(spec, 1641970, CloudMismatch)
    return spec


class TestIterClouds(FakeHomeTestCase):

    bogus_type = cloud_spec('bogus-type', 'bogus-type', {'type': 'bogus'},
                            exception=TypeNotAccepted)

    def test_manual(self):
        self.maxDiff = None
        cloud = {'type': 'manual', 'endpoint': 'http://example.com'}
        spec = cloud_spec('foo', 'foo', cloud)
        self.assertItemsEqual([
            self.bogus_type, spec,
            xfail(cloud_spec('long-name-foo', long_text, cloud),
                  1641970, NameMismatch),
            xfail(cloud_spec('invalid-name-foo', 'invalid/name', cloud,
                             exception=NameNotAccepted), 1641981, None),
            make_long_endpoint(spec, basic_validation)
            ],
            iter_clouds({'foo': cloud}, endpoint_validation))

    def test_manual_no_validation(self):
        self.maxDiff = None
        cloud = {'type': 'manual', 'endpoint': 'http://example.com'}
        spec = cloud_spec('foo', 'foo', cloud)
        self.assertItemsEqual([
            self.bogus_type, spec,
            xfail(cloud_spec('long-name-foo', long_text, cloud),
                  1641970, NameMismatch),
            xfail(cloud_spec('invalid-name-foo', 'invalid/name', cloud,
                             exception=NameNotAccepted), 1641981, None),
            make_long_endpoint(
                spec, basic_validation)
            ],
            iter_clouds({'foo': cloud}, basic_validation))

    def test_vsphere(self):
        cloud = {
            'type': 'vsphere',
            'endpoint': '1.2.3.4',
            'regions': {'q': {'endpoint': '1.2.3.4'}},
            }
        spec = cloud_spec('foo', 'foo', cloud, exception=None)
        self.assertItemsEqual([
            self.bogus_type, spec,
            xfail(cloud_spec('invalid-name-foo', 'invalid/name', cloud,
                             exception=NameNotAccepted), 1641981, None),
            xfail(cloud_spec('long-name-foo', long_text, cloud,
                             exception=None), 1641970, NameMismatch),
            make_long_endpoint(
                spec, endpoint_validation, regions=True),
            ], iter_clouds({'foo': cloud}, endpoint_validation))

    def test_vsphere_no_validation(self):
        cloud = {
            'type': 'vsphere',
            'endpoint': '1.2.3.4',
            'regions': {'q': {'endpoint': '1.2.3.4'}},
            }
        spec = cloud_spec('foo', 'foo', cloud, exception=None)
        self.assertItemsEqual([
            self.bogus_type, spec,
            xfail(cloud_spec('invalid-name-foo', 'invalid/name', cloud,
                             exception=NameNotAccepted), 1641981, None),
            xfail(cloud_spec('long-name-foo', long_text, cloud,
                             exception=None), 1641970, NameMismatch),
            xfail(make_long_endpoint(spec,
                                     endpoint_validation, regions=True),
                  1641970, CloudMismatch),
            ], iter_clouds({'foo': cloud}, basic_validation))

    def test_maas(self):
        cloud = {
            'type': 'maas',
            'endpoint': 'http://example.com',
            }
        spec = cloud_spec('foo', 'foo', cloud, exception=None)
        self.assertItemsEqual([
            self.bogus_type, spec,
            xfail(cloud_spec('invalid-name-foo', 'invalid/name', cloud,
                             exception=NameNotAccepted), 1641981, None),
            xfail(cloud_spec('long-name-foo', long_text, cloud,
                             exception=None), 1641970, NameMismatch),
            make_long_endpoint(spec, endpoint_validation),
            ], iter_clouds({'foo': cloud}, endpoint_validation))

    def test_maas_no_validation(self):
        cloud = {
            'type': 'maas',
            'endpoint': 'http://example.com',
            }
        spec = cloud_spec('foo', 'foo', cloud, exception=None)
        self.assertItemsEqual([
            self.bogus_type, spec,
            xfail(cloud_spec('invalid-name-foo', 'invalid/name', cloud,
                             exception=NameNotAccepted), 1641981, None),
            xfail(cloud_spec('long-name-foo', long_text, cloud,
                             exception=None), 1641970, NameMismatch),
            make_long_endpoint(spec, basic_validation),
            ], iter_clouds({'foo': cloud}, basic_validation))

    def test_openstack(self):
        config = {'type': 'openstack', 'endpoint': 'http://example.com',
                  'regions': {'bar': {'endpoint': 'http://baz.example.com'}}}
        spec = cloud_spec('foo', 'foo', config, exception=None)
        invalid_name = xfail(
            cloud_spec('invalid-name-foo', 'invalid/name', config,
                       exception=NameNotAccepted), 1641981, None)
        long_name = xfail(
            cloud_spec('long-name-foo', long_text, config, exception=None),
            1641970, NameMismatch)
        long_region = cloud_spec(
            'long-endpoint-foo-bar', 'foo', deepcopy(config), InvalidEndpoint)
        long_region.config['regions']['bar']['endpoint'] = long_text
        bogus_auth = cloud_spec('bogus-auth-foo', 'foo',
                                deepcopy(config), exception=AuthNotAccepted)
        bogus_auth.config['auth-types'] = ['asdf']
        self.assertItemsEqual([
            self.bogus_type, spec, invalid_name, long_name, long_region,
            bogus_auth,
            make_long_endpoint(spec, endpoint_validation),
            ], iter_clouds({'foo': config}, endpoint_validation))

    def test_openstack_no_validation(self):
        config = {'type': 'openstack', 'endpoint': 'http://example.com',
                  'regions': {'bar': {'endpoint': 'http://baz.example.com'}}}
        spec = cloud_spec('foo', 'foo', config, exception=None)
        invalid_name = xfail(
            cloud_spec('invalid-name-foo', 'invalid/name', config,
                       exception=NameNotAccepted), 1641981, None)
        long_name = xfail(
            cloud_spec('long-name-foo', long_text, config, exception=None),
            1641970, NameMismatch)
        long_region = xfail(cloud_spec(
            'long-endpoint-foo-bar', 'foo', deepcopy(config),
            InvalidEndpoint), 1641970, CloudMismatch)
        long_region.config['regions']['bar']['endpoint'] = long_text
        bogus_auth = cloud_spec('bogus-auth-foo', 'foo',
                                deepcopy(config), exception=AuthNotAccepted)
        bogus_auth.config['auth-types'] = ['asdf']
        self.assertItemsEqual([
            self.bogus_type, spec, invalid_name, long_name, long_region,
            bogus_auth,
            make_long_endpoint(spec, basic_validation),
            ], iter_clouds({'foo': config}, basic_validation))


class TestAssessAllClouds(FakeHomeTestCase):

    def test_assess_all_clouds(self):
        client = fake_juju_client(juju_home=self.juju_home)
        clouds = {'a': {'type': 'foo'}, 'b': {'type': 'bar'}}
        cloud_specs = iter_clouds(clouds, endpoint_validation)
        exception = Exception()
        with patch('assess_add_cloud.assess_cloud',
                   side_effect=[TypeNotAccepted(), None] + [exception] * 7):
            with patch('sys.stdout'):
                with patch('logging.exception') as exception_mock:
                    succeeded, xfail, failed = assess_all_clouds(client,
                                                                 cloud_specs)
        self.assertEqual({'bogus-type', 'a'}, succeeded)
        self.assertEqual({
            'b', 'bogus-auth-a', 'bogus-auth-b', 'invalid-name-a',
            'invalid-name-b', 'long-name-a', 'long-name-b'},
            failed)
        self.assertEqual(exception_mock.mock_calls, [call(exception)] * 7)

    def test_xfail(self):
        cloud_specs = [xfail(cloud_spec('label1', 'name1', {'config': '1'}),
                             27, TypeNotAccepted)]
        client = fake_juju_client(juju_home=self.juju_home)
        with patch('assess_add_cloud.assess_cloud',
                   side_effect=TypeNotAccepted):
            with patch('logging.exception') as exception_mock:
                with patch('sys.stdout'):
                    succeeded, xfailed, failed = assess_all_clouds(client,
                                                                   cloud_specs)
        self.assertEqual(set(), failed)
        self.assertEqual({27: {'label1'}}, xfailed)
        self.assertEqual(0, exception_mock.call_count)

    def test_failed_notraised(self):
        client = fake_juju_client(juju_home=self.juju_home)
        cloud_specs = [
            cloud_spec('label', 'name', {'config': '1'}, TypeNotAccepted)]
        with patch('assess_add_cloud.assess_cloud'):
            with patch('logging.exception') as exception_mock:
                with patch('sys.stdout'):
                    succeeded, xfailed, failed = assess_all_clouds(client,
                                                                   cloud_specs)
        self.assertEqual(set(['label']), failed)
        self.assertEqual(1, exception_mock.call_count)
        raised_e = exception_mock.mock_calls[0][1][0]
        self.assertEqual(
            "Expected exception not raised: "
            "<class 'jujupy.client.TypeNotAccepted'>",
            raised_e.message)


class TestWriteStatus(FakeHomeTestCase):

    def do_write(self, status, items):
        stdout = StringIO()
        with patch('sys.stdout', stdout):
            write_status(status, items)
        return stdout.getvalue()

    def test_write_none(self):
        self.assertEqual('pending: none\n', self.do_write('pending', set()))

    def test_write_one(self):
        self.assertEqual('pending: q\n', self.do_write('pending', {'q'}))

    def test_write_two(self):
        self.assertEqual('pending: q, r\n',
                         self.do_write('pending', {'r', 'q'}))
