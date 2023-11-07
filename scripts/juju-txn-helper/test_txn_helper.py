import unittest
from unittest import mock

from bson import ObjectId

import txn_helper


class TestGetModelUuid(unittest.TestCase):

    def test_valid_uuid(self):
        model_uuid = "01234567-89ab-cdef-0123-456789abcdef"
        args = mock.Mock()
        args.model = model_uuid
        result = txn_helper.get_model_uuid(None, args)
        self.assertEqual(result, model_uuid)

    def test_name_found(self):
        model_name = "foo"
        model_uuid = "01234567-89ab-cdef-0123-456789abcdef"

        client = mock.MagicMock()
        client.juju.models.find_one.return_value = {"_id": model_uuid}
        args = mock.Mock()
        args.model = model_name

        result = txn_helper.get_model_uuid(client, args)
        self.assertEqual(result, model_uuid)

    def test_name_not_found(self):
        model_name = "foo"

        client = mock.MagicMock()
        client.juju.models.find_one.return_value = None
        args = mock.Mock()
        args.model = model_name

        with self.assertRaises(SystemExit) as cm:
            txn_helper.get_model_uuid(client, args)
        self.assertEqual(cm.exception.code, "Could not find the specified model (foo)")


class BaseTxnQueueTest(unittest.TestCase):
    SAMPLE_ABORTED_TRANSACTION = {
        "_id": ObjectId("0123456789abcdef01234567"),
        "s": 5,
        "o": [
            {
                "c": "models",
                "d": "01234567-891b-cdef-0123-456789abcdef",
                "a": {
                    "life": 0,
                    "migration-mode": ""
                }
            },
            {
                "c": "remoteApplications",
                "d": "01234567-891b-cdef-0123-456789abcdef:remote-0123456789abcdef0123456789abcdef",
                "a": "d-",
                "i": {
                    "_id": "01234567-891b-cdef-0123-456789abcdef:remote-0123456789abcdef0123456789abcdef",
                    "name": "remote-0123456789abcdef0123456789abcdef",
                    "offer-uuid": "01010101-0101-0101-0101-010101010101",
                    "source-model-uuid": "23232323-2323-2323-2323-232323232323",
                    "endpoints": [
                        {
                            "name": "monitors",
                            "role": "provider",
                            "interface": "monitors",
                            "limit": 0,
                            "scope": ""
                        }
                    ],
                    "spaces": [],
                    "bindings": {

                    },
                    "life": 0,
                    "relationcount": 0,
                    "is-consumer-proxy": True,
                    "version": 0,
                    "model-uuid": "01234567-891b-cdef-0123-456789abcdef"
                }
            },
            {
                "c": "applications",
                "d": "01234567-891b-cdef-0123-456789abcdef:remote-0123456789abcdef0123456789abcdef",
                "a": "d-"
            },
            {
                "c": "remoteEntities",
                "d": "01234567-891b-cdef-0123-456789abcdef:application-remote-0123456789abcdef0123456789abcdef",
                "a": "d-",
                "i": {
                    "_id": "01234567-891b-cdef-0123-456789abcdef:",
                    "token": "45454545-4545-4545-4545-454545454545",
                    "model-uuid": "01234567-891b-cdef-0123-456789abcdef"
                }
            }
        ],
        "n": "fedcba98"
    }


class TestWalkTxnQueue(BaseTxnQueueTest):

    SAMPLE_TRANSACTION_REFERENCES = [
        "0123456789abcdef01234567_78787878",
    ]
    SAMPLE_TRANSACTIONS = [
        BaseTxnQueueTest.SAMPLE_ABORTED_TRANSACTION,
    ]

    expected_stdout = """\
Retrieved model transaction queue:
- 0123456789abcdef01234567_78787878
Converting to transaction IDs:
- 0123456789abcdef01234567

Transaction 0123456789abcdef01234567 (state: aborted):
  Transaction dump:
    {'_id': ObjectId('0123456789abcdef01234567'),
     'n': 'fedcba98',
     'o': [{'a': {'life': 0, 'migration-mode': ''},
            'c': 'models',
            'd': '01234567-891b-cdef-0123-456789abcdef'},
           {'a': 'd-',
            'c': 'remoteApplications',
            'd': '01234567-891b-cdef-0123-456789abcdef:remote-0123456789abcdef0123456789abcdef',
            'i': {'_id': '01234567-891b-cdef-0123-456789abcdef:remote-0123456789abcdef0123456789abcdef',
                  'bindings': {},
                  'endpoints': [{'interface': 'monitors',
                                 'limit': 0,
                                 'name': 'monitors',
                                 'role': 'provider',
                                 'scope': ''}],
                  'is-consumer-proxy': True,
                  'life': 0,
                  'model-uuid': '01234567-891b-cdef-0123-456789abcdef',
                  'name': 'remote-0123456789abcdef0123456789abcdef',
                  'offer-uuid': '01010101-0101-0101-0101-010101010101',
                  'relationcount': 0,
                  'source-model-uuid': '23232323-2323-2323-2323-232323232323',
                  'spaces': [],
                  'version': 0}},
           {'a': 'd-',
            'c': 'applications',
            'd': '01234567-891b-cdef-0123-456789abcdef:remote-0123456789abcdef0123456789abcdef'},
           {'a': 'd-',
            'c': 'remoteEntities',
            'd': '01234567-891b-cdef-0123-456789abcdef:application-remote-0123456789abcdef0123456789abcdef',
            'i': {'_id': '01234567-891b-cdef-0123-456789abcdef:',
                  'model-uuid': '01234567-891b-cdef-0123-456789abcdef',
                  'token': '45454545-4545-4545-4545-454545454545'}}],
     's': 5}

  Op 0: QUERY_DOC assertion passed
  Collection 'models', ID '01234567-891b-cdef-0123-456789abcdef'
  Query doc tested was:
    {'_id': '01234567-891b-cdef-0123-456789abcdef', 'life': 0, 'migration-mode': ''}
  Existing doc is:
    {'txn-queue': ['0123456789abcdef01234567_78787878']}

  Op 1: DOC_MISSING assertion FAILED
  Collection 'remoteApplications', ID '01234567-891b-cdef-0123-456789abcdef:remote-0123456789abcdef0123456789abcdef'
  Existing doc is:
    {'dummy': 'remoteApplication'}
  Insert doc is:
    {'_id': '01234567-891b-cdef-0123-456789abcdef:remote-0123456789abcdef0123456789abcdef',
     'bindings': {},
     'endpoints': [{'interface': 'monitors',
                    'limit': 0,
                    'name': 'monitors',
                    'role': 'provider',
                    'scope': ''}],
     'is-consumer-proxy': True,
     'life': 0,
     'model-uuid': '01234567-891b-cdef-0123-456789abcdef',
     'name': 'remote-0123456789abcdef0123456789abcdef',
     'offer-uuid': '01010101-0101-0101-0101-010101010101',
     'relationcount': 0,
     'source-model-uuid': '23232323-2323-2323-2323-232323232323',
     'spaces': [],
     'version': 0}

  Op 2: DOC_MISSING assertion FAILED
  Collection 'applications', ID '01234567-891b-cdef-0123-456789abcdef:remote-0123456789abcdef0123456789abcdef'
  Existing doc is:
    {'dummy': 'application'}

  Op 3: DOC_MISSING assertion FAILED
  Collection 'remoteEntities', ID '01234567-891b-cdef-0123-456789abcdef:application-remote-0123456789abcdef0123456789abcdef'
  Existing doc is:
    {'dummy': 'remoteEntity'}
  Insert doc is:
    {'_id': '01234567-891b-cdef-0123-456789abcdef:',
     'model-uuid': '01234567-891b-cdef-0123-456789abcdef',
     'token': '45454545-4545-4545-4545-454545454545'}

"""

    @mock.patch('txn_helper.print')
    def test_happy_path_single_match(self, print_mock: mock.MagicMock):
        client = mock.MagicMock()

        # In the single transaction case, there is next to no difference between querying for a model or going over the
        # full set of transactions since in either way we control the returned data via a mock.
        # We'll use the model query path here for the sake of covering slightly more code.
        model_uuid = "01234567-89ab-cdef-0123-456789abcdef"
        state_filter = None

        dump_transaction = True
        include_passes = True
        max_transaction_count = None

        # Mock out the model to include a model transaction queue
        client.juju.models.find_one.return_value = {"txn-queue": self.SAMPLE_TRANSACTION_REFERENCES}
        # Mock the result of the transaction query
        client.juju.txns.find.return_value = self.SAMPLE_TRANSACTIONS
        # Mock out other specific objects checked by the assertions
        client.juju.remoteApplications.find_one.return_value = {"dummy": "remoteApplication"}
        client.juju.applications.find_one.return_value = {"dummy": "application"}
        client.juju.remoteEntities.find_one.return_value = {"dummy": "remoteEntity"}

        txn_queue = txn_helper.get_model_transaction_queue(client, model_uuid)
        txn_helper.walk_transaction_queue(client, txn_queue, state_filter, dump_transaction, include_passes, max_transaction_count)

        captured_stdout = "\n".join([call.args[0] if call.args else "" for call in print_mock.mock_calls])

        # Yes, this is a very fragile test - any change to stdout can make it fail - but it will work for now.
        self.assertEqual(captured_stdout, self.expected_stdout)

    # Consider adding:
    # * Model query with multiple transactions - this would result in iterating over multiple distinct queries' cursors.
    # * All transactions query with mulitple transactions - this would result in iterating over a single query's cursor.
    # (The above paths have been directly tested on a test database.)
