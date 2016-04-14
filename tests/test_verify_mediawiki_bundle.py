import pickle
import urllib2
from mock import (
    call,
    Mock,
    patch,
)

from assess_min_version import JujuAssertionError
import tests
from tests.test_jujupy import FakeJujuClient
import verify_mediawiki_bundle
from verify_mediawiki_bundle import(
    assess_mediawiki_bundle,
    _get_ssl_ctx,
    parse_args,
    verify_services,
    wait_for_http,
)


class TestVerifyMediaWikiBundle(tests.TestCase):

    def test_parse_args(self):
        client = {'a': 'foo'}
        client_ser = pickle.dumps(client)
        args = parse_args([client_ser])
        self.assertEqual(args.client, client)
        self.assertEqual(args.verbose, 20)

    def test_wait_for_http(self):
        fake_res = FakeResponse()
        with patch('verify_mediawiki_bundle._get_ssl_ctx', autospec=True,
                   return_value=None) as ssl_mock:
            with patch('urllib2.urlopen', autospec=True,
                       return_value=fake_res) as uo_mock:
                wait_for_http("example.com")
        uo_mock.assert_called_once_with('example.com')
        ssl_mock.assert_called_once_with()

    def test_wait_for_http_ssl_ctx(self):
        fake_res = FakeResponse()
        with patch('verify_mediawiki_bundle._get_ssl_ctx', autospec=True,
                   return_value=FakeSslCtx()) as ssl_mock:
            with patch('urllib2.urlopen', return_value=fake_res) as uo_mock:
                        wait_for_http("example.com")
        uo_mock.assert_called_once_with(
            'example.com', context=ssl_mock.return_value)
        ssl_mock.assert_called_once_with()

    def test_wait_for_http_timeout(self):
        fake_res = FakeResponse()
        with patch('verify_mediawiki_bundle._get_ssl_ctx', autospec=True,
                   return_value=None) as ssl_mock:
            with patch('urllib2.urlopen', autospec=True,
                       return_value=fake_res):
                with self.assertRaisesRegexp(
                        JujuAssertionError, 'example.com is not reachable'):
                    wait_for_http("example.com", timeout=0)
        ssl_mock.assert_called_once_with()

    def test_wait_for_http_httperror(self):
        with patch('verify_mediawiki_bundle._get_ssl_ctx', autospec=True,
                   return_value=None) as ssl_mock:
            with patch('urllib2.urlopen', autospec=True,
                       return_value=urllib2.HTTPError(
                           None, None, None, None, None)) as uo_mock:
                with patch('verify_mediawiki_bundle.until_timeout',
                           autospec=True, return_value=[1]) as ut_mock:
                    with self.assertRaisesRegexp(
                            JujuAssertionError,
                            'example.com is not reachable'):
                        wait_for_http("example.com")
        uo_mock.assert_called_once_with('example.com')
        self.assertEqual(ut_mock.mock_calls, [call(600)])
        ssl_mock.assert_called_once_with()

    def test_verify_services(self):
        client = self.deploy_mediawiki()
        fake_res = FakeResponse()
        services = ['haproxy', 'mediawiki', 'mysql', 'memcached',
                    'mysql-slave']
        with patch('urllib2.urlopen', autospec=True,
                   return_value=fake_res) as url_mock:
            with patch('verify_mediawiki_bundle._get_ssl_ctx', autospec=True,
                       return_value=None) as ssl_mock:
                verify_services(client, services, text='foo')
        url_mock.assert_called_once_with('http://1.example.com')
        ssl_mock.assert_called_once_with()

    def test_verify_services_haproxy_exposed_by_default(self):
        client = self.deploy_mediawiki()
        fake_res = FakeResponse()
        services = ['haproxy', 'mediawiki', 'mysql', 'memcached',
                    'mysql-slave']
        client.juju('expose', ('haproxy',))
        with patch('urllib2.urlopen', autospec=True,
                   return_value=fake_res) as url_mock:
            with patch('verify_mediawiki_bundle._get_ssl_ctx', autospec=True,
                       return_value=None) as ssl_mock:
                verify_services(client, services, text='foo',
                                haproxy_exposed=True)
        url_mock.assert_called_once_with('http://1.example.com')
        ssl_mock.assert_called_once_with()

    def test_verify_service_misconfigured(self):
        client = FakeJujuClient()
        client.bootstrap()
        client.deploy('haproxy')
        client.deploy('mysql')
        services = ['haproxy', 'mediawiki', 'mysql', 'memcached',
                    'mysql-slave']
        with self.assertRaisesRegexp(
                JujuAssertionError, 'Unexpected service configuration'):
            verify_services(client, services)

    def test_verify_services_haproxy_exposed(self):
        client = self.deploy_mediawiki()
        services = ['haproxy', 'mediawiki', 'mysql', 'memcached',
                    'mysql-slave']
        client.juju('expose', ('haproxy',))
        with self.assertRaisesRegexp(
                JujuAssertionError, 'haproxy is exposed.'):
            verify_services(client, services)

    def test_verify_services_haproxy_not_exposed(self):
        client = self.deploy_mediawiki()
        services = ['haproxy', 'mediawiki', 'mysql', 'memcached',
                    'mysql-slave']
        client.juju = Mock(spec=[])
        with self.assertRaisesRegexp(
                JujuAssertionError, 'haproxy is not exposed.'):
            verify_services(client, services)

    def test_verify_services_text_not_found(self):
        client = self.deploy_mediawiki()
        services = ['haproxy', 'mediawiki', 'mysql', 'memcached',
                    'mysql-slave']
        fake_res = FakeResponse()
        with patch('urllib2.urlopen', autospec=True,
                   return_value=fake_res) as url_mock:
            with self.assertRaisesRegexp(
                JujuAssertionError,
                    "landscape is not found in 1.example.com"):
                verify_services(client, services, text="landscape")
        url_mock.assert_called_once_with("http://1.example.com")

    def test_assert_mediawiki_bundle(self):
        client = self.deploy_mediawiki()
        fake_res = FakeResponse()
        services = ['haproxy', 'mediawiki', 'mysql', 'memcached',
                    'mysql-slave']
        with patch('verify_mediawiki_bundle.verify_services', autospec=True,
                   return_value=fake_res) as vs_mock:
            assess_mediawiki_bundle(client)
        vs_mock.assert_called_once_with(client, services)

    def test_get_ssl_ctx(self):
        ssl_mock = Mock(spec=['create_default_context'], CERT_NONE=0)
        ssl_mock.create_default_context.return_value = FakeSslCtx()
        verify_mediawiki_bundle.ssl = ssl_mock
        ctx = _get_ssl_ctx()
        self.assertIs(ctx.check_hostname, False)
        self.assertEqual(ctx.verify_mode, 0)

    def test_get_ssl_ctx_none(self):
        verify_mediawiki_bundle.ssl = Mock(spec=[])
        ctx = _get_ssl_ctx()
        self.assertIsNone(ctx)

    def deploy_mediawiki(self):
        client = FakeJujuClient()
        client.bootstrap()
        client.deploy('haproxy')
        client.deploy('mediawiki')
        client.deploy('mysql')
        client.deploy('memcached')
        client.deploy('mysql-slave')
        return client


class FakeSslCtx:
    check_hostname = None
    verify_mode = None


class FakeSslOld:
    pass


class FakeResponse:

    def __init__(self, code=200):
        self.return_code = code

    def getcode(self):
        return self.return_code

    def read(self):
        return "foo"
