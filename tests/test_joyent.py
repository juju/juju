from datetime import (
    datetime,
    timedelta,
)
from mock import patch
from unittest import TestCase

from joyent import (
    Client,
    ISO_8601_FORMAT,
    parse_args,
)


class JoyentTestCase(TestCase):

    def test_parse_args(self):
        args = parse_args(
            ['-d', '-v', '-u', 'sdc_url', '-a', 'account', '-k', 'key_id',
             '-p', 'key/path', 'list-machines'])
        self.assertEqual('sdc_url', args.sdc_url)
        self.assertEqual('account', args.account)
        self.assertEqual('key_id', args.key_id)
        self.assertEqual('key/path', args.key_path)
        self.assertTrue(args.dry_run)
        self.assertTrue(args.verbose)


class ClientTestCase(TestCase):

    def test_init(self):
        client = Client(
            'sdc_url', 'account', 'key_id', './key',
            dry_run=True, verbose=True)
        self.assertEqual('sdc_url', client.sdc_url)
        self.assertEqual('account', client.account)
        self.assertEqual('key_id', client.key_id)
        self.assertEqual('./key', client.key_path)
        self.assertEqual(3, client.pause)
        self.assertTrue(client.dry_run)
        self.assertTrue(client.verbose)

    def test_delete_old_machines(self):
        then = datetime.utcnow() - timedelta(hours=2)
        machine = {
            'id': 'id',
            'state': 'stopped',
            'created': then.strftime(ISO_8601_FORMAT)}

        def fake_list_machines(id=None):
            if id:
                return machine
            else:
                return [machine]

        client = Client('sdc_url', 'account', 'key_id', './key', pause=0)
        with patch.object(client, '_list_machines',
                          side_effect=fake_list_machines) as lm_mock:
            with patch.object(client, 'stop_machine') as sm_mock:
                with patch.object(client, 'delete_machine') as dm_mock:
                    client.delete_old_machines(1, 'foo@bar')
        lm_mock.assert_call_any(None)
        lm_mock.assert_call_any('id')
        sm_mock.assert_called_once_with('id')
        dm_mock.assert_called_once_with('id')
