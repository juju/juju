#!/usr/bin/env python3
# Copyright Canonical Ltd.
# Licensed under the GNU General Affero Public License version 3.0.

"""txn_helper.py - A model transaction queue analysis tool"""

import argparse
import pprint
import re
import sys
import textwrap
from enum import Enum

from bson import ObjectId
from pymongo import MongoClient


class OpState(Enum):
    """Juju transaction operation states."""
    PREPARING = 1
    PREPARED = 2
    ABORTING = 3
    APPLYING = 4
    ABORTED = 5
    APPLIED = 6


class AssertionType(Enum):
    """Transaction assertion types.

    Note that these are simply names used for convenience in this script;
    DOC_EXISTS/DOC_MISSING refer to the d+/d- assertion codes, while QUERY_DOC
    refers to assertions which specify MongoDB query documents.
    """
    DOC_EXISTS = 1
    DOC_MISSING = 2
    QUERY_DOC = 3


STATE_MAP = {enum_entry.value: enum_entry.name.lower() for enum_entry in OpState}

DOC_EXISTS = "d+"
DOC_MISSING = "d-"

UUID_REGEX = re.compile(
    r"^[\dA-Fa-f]{8}-[\dA-Fa-f]{4}-[\dA-Fa-f]{4}-[\dA-Fa-f]{4}-[\dA-Fa-f]{12}$")


def main():
    """The main program entry point."""
    args = parse_args()
    client_args = create_client_args(args)
    client = MongoClient(**client_args)
    if args.model:
        # Target all transactions from the specified model's txn-queue.
        model_uuid = get_model_uuid(client, args)
        txn_queue = get_model_transaction_queue(client, model_uuid)
        state_filter = None
    else:
        # Default behavior is to filter by transaction state, usually on the "aborted" state.
        txn_queue = None
        state_filter = args.state_filter
    walk_transaction_queue(client, txn_queue, state_filter, args.dump_transaction, args.include_passes, args.count)


def parse_args():
    """Parse the command line arguments."""
    ap = argparse.ArgumentParser()    # pylint: disable=invalid-name
    ap.add_argument('model', nargs='?',
                    help='Name or UUID of model to examine.  If not specified, the full '
                         'transaction collection will be examined instead.')
    ap.add_argument('-H', '--host', help='Mongo hostname or URI to use, if required.')
    ap.add_argument('-u', '--user', help='Mongo username to use, if required.')
    ap.add_argument('-p', '--password', help='Mongo password to use, if required.')
    ap.add_argument('--auth-database', default='admin',
                    help='Mongo auth database to use, if required.  (Default: %(default)s)')
    ap.add_argument('--state', dest='state_filter', type=int, default=OpState.ABORTED.value,
                    help="Filter by state number.  This is used when querying the full "
                         "transaction queue; it is ignored if querying a specific object's "
                         "transaction queue.  (Default: %(default)s)")
    ap.add_argument('-c', '--count', type=int, help='Count of entries to examine')
    ap.add_argument('-d', '--dump-transaction', action='store_true',
                    help='Additionally pretty-print entire transactions to stdout')
    ap.add_argument('-P', '--include-passes', action='store_true', help='Include pass details')
    ap.add_argument('-s', '--ssl', '--tls', dest='tls', action='store_true', help='Enable TLS')
    return ap.parse_args()


def create_client_args(args):
    """Create a set of client arguments suitable for talking to the target Mongo instance."""
    client_args = {}
    if args.host:
        client_args['host'] = args.host
    if args.user:
        client_args['username'] = args.user
        client_args['authSource'] = args.auth_database
    if args.password:
        client_args['password'] = args.password
    if args.tls:
        client_args['tls'] = True
        client_args['tlsAllowInvalidCertificates'] = True
    return client_args


def get_model_uuid(client, args):
    """Given a model argument, convert it (if necessary) to a model UUID."""
    model = args.model
    print("Examining supplied model:", model)
    if UUID_REGEX.match(model):
        model_uuid = model
        print('Supplied model is a valid UUID; using as-is.')
    else:
        model_doc = client.juju.models.find_one({'name': model})
        if model_doc:
            model_uuid = model_doc['_id']
            print('Found matching UUID:', model_uuid)
        else:
            sys.exit('Could not find the specified model ({})'.format(model))
    return model_uuid


def get_model_transaction_queue(client, model_uuid):
    """Retrieves a list of transaction IDs from the specified model's document."""
    model_doc = client.juju.models.find_one({"_id": model_uuid})
    if not model_doc:
        sys.exit('Could not find model with specified UUID ({})'.format(model_uuid))
    txn_queue = model_doc['txn-queue']
    print('Retrieved model transaction queue:')
    for ref in txn_queue:
        print('- {}'.format(ref))
    print('Converting to transaction IDs:')
    txn_ids = [ref.split("_")[0] for ref in txn_queue]
    for id_ in txn_ids:
        print('- {}'.format(id_))
    print()
    return txn_ids


def walk_transaction_queue(client, txn_queue, state_filter, dump_transaction, include_passes, max_transaction_count):
    """Examine part or all of the transactions collection."""
    db_client = client.juju
    state_filter_args = {} if state_filter is None else {"s": state_filter}
    if txn_queue:
        # We'll perform one query for each specific transaction ID
        query_docs = [dict(state_filter_args, _id=ObjectId(txn)) for txn in txn_queue]
    else:
        # We'll use a single cursor and iterate over the full collection of transactions
        query_docs = [state_filter_args]

    txn_counter = 0
    for query_doc in query_docs:
        matches = db_client.txns.find(query_doc)
        for match in matches:
            print('Transaction {} (state: {}):'.format(str(match['_id']), get_state_as_string(match['s'])))
            if dump_transaction:
                print('  Transaction dump:\n{}'.format(textwrap.indent(pprint.pformat(match), '    ')))
                print()
            for op_index, op in enumerate(match['o']):                      # pylint: disable=invalid-name
                _print_op_details(db_client, op_index, op, include_passes)  # pylint: disable=invalid-name
            txn_counter += 1
            print()

            if max_transaction_count and txn_counter >= max_transaction_count:
                break
        if max_transaction_count and txn_counter >= max_transaction_count:
            break


def get_state_as_string(i):
    """Given an integer transaction state, return its meaning as a string."""
    return STATE_MAP[i]


def _print_op_details(db_client, op_index, op, include_passes):  # pylint: disable=invalid-name
    collection = getattr(db_client, op['c'])
    match_doc_id = op['d']
    find_filter = {"_id": match_doc_id}
    if 'a' not in op:
        if include_passes:
            print('  Op {}: no assertion present; passes'.format(op_index))
    else:
        if op['a'] == DOC_MISSING:
            assertion_type = AssertionType.DOC_MISSING
            existing_doc = collection.find_one(find_filter)
            failed = existing_doc
        elif op['a'] == DOC_EXISTS:
            assertion_type = AssertionType.DOC_EXISTS
            existing_doc = collection.find_one(find_filter)
            failed = not existing_doc
        else:
            # Standard assertion.  Uses a query doc.
            assertion_type = AssertionType.QUERY_DOC
            find_filter.update(op['a'])
            existing_doc = collection.find_one(find_filter)
            failed = not existing_doc

        should_print = failed or include_passes
        if should_print:
            print('  Op {}: {} assertion {}'.format(op_index, assertion_type.name, 'FAILED' if failed else 'passed'))
            print("  Collection '{}', ID '{}'".format(op['c'], match_doc_id))
            if assertion_type == AssertionType.QUERY_DOC:
                print(
                    '  Query doc tested was:\n{}'.format(textwrap.indent(pprint.pformat(find_filter), '    ')))
            if existing_doc:
                print('  Existing doc is:\n{}'.format(textwrap.indent(pprint.pformat(existing_doc), '    ')))
            if 'i' in op:
                print('  Insert doc is:\n{}'.format(textwrap.indent(pprint.pformat(op['i']), '    ')))
            if 'u' in op:
                print('  Update doc is:\n{}'.format(textwrap.indent(pprint.pformat(op['u']), '    ')))
    if should_print:
        print()


if __name__ == "__main__":
    main()
