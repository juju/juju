#!/usr/bin/env python3
# Copyright Canonical Ltd.
# Licensed under the GNU General Affero Public License version 3.0.

"""txn_helper.py - A model transaction queue analysis tool"""

import argparse
import pprint
import re
import sys
import textwrap

from bson import ObjectId
from pymongo import MongoClient


STATE_MAP = {
        1: "preparing",
        2: "prepared",
        3: "aborting",
        4: "applying",
        5: "aborted",
        6: "applied",
        }

DOC_EXISTS = "d+"
DOC_MISSING = "d-"

UUID_REGEX = re.compile(
    r"^[\dA-Fa-f]{8}-[\dA-Fa-f]{4}-[\dA-Fa-f]{4}-[\dA-Fa-f]{4}-[\dA-Fa-f]{12}$")


def main():
    """The main program entry point."""
    args = parse_args()
    client_args = create_client_args(args)
    client = MongoClient(**client_args)
    model_uuid = get_model_uuid(client, args)
    walk_transaction_queue(client, model_uuid, args.dump_transaction, args.include_passes, args.count)


def parse_args():
    """Parse the command line arguments."""
    ap = argparse.ArgumentParser()    # pylint: disable=invalid-name
    ap.add_argument('model', help='Name or UUID of model to examine')
    ap.add_argument('-H', '--host', help='Mongo hostname or URI to use, if required.')
    ap.add_argument('-u', '--user', help='Mongo username to use, if required.')
    ap.add_argument('-p', '--password', help='Mongo password to use, if required.')
    ap.add_argument('--auth-database', default='admin',
                    help='Mongo auth database to use, if required.  (Default: %(default)s)')
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
    if UUID_REGEX.match(args.model):
        model_uuid = args.model
    else:
        model_doc = client.juju.models.find_one({'name': args.model})
        if model_doc:
            model_uuid = model_doc['_id']
        else:
            sys.exit('Could not find the specified model ({})'.format(args.model))
    return model_uuid


def walk_transaction_queue(client, model_uuid, dump_transaction, include_passes, max_transaction_count):
    """Walk through the model's transaction queue, printing details about each transaction."""
    db_client = client.juju
    model_doc = db_client.models.find_one({"_id": model_uuid})
    if not model_doc:
        sys.exit('Could not find model with specified UUID ({})'.format(model_uuid))
    txn_queue = model_doc['txn-queue']
    for txn_index, txn in enumerate(txn_queue):
        if max_transaction_count and txn_index >= max_transaction_count:
            break
        id_ = txn.split('_')[0]   # discard the suffix (probably the nonce field?)
        matches = list(db_client.txns.find({"_id": ObjectId(id_)}))
        for match_index, match in enumerate(matches):
            print('TXN {} found: {} (state: {}):'.format(txn_index, id_, get_state_as_string(match['s'])))
            if match_index > 0:
                # Probably will never happen, but just in case
                print('WARNING: multiple transactions with the same transaction ID detected')
            if dump_transaction:
                print('  Transaction dump:\n{}'.format(textwrap.indent(pprint.pformat(match), '    ')))
            for op_index, op in enumerate(match['o']):                      # pylint: disable=invalid-name
                _print_op_details(db_client, op_index, op, include_passes)  # pylint: disable=invalid-name
            print()


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
            assertion_type = 'DOC_MISSING'
            existing_doc = collection.find_one(find_filter)
            failed = existing_doc
        elif op['a'] == DOC_EXISTS:
            assertion_type = 'DOC_EXISTS'
            existing_doc = collection.find_one(find_filter)
            failed = not existing_doc
        else:
            # Standard assertion.  Uses a query doc.
            assertion_type = 'query-doc-style'
            find_filter.update(op['a'])
            existing_doc = collection.find_one(find_filter)
            failed = not existing_doc

        should_print = failed or include_passes
        if should_print:
            print('  Op {}: {} assertion {}'.format(op_index, assertion_type, 'FAILED' if failed else 'passed'))
            print('  Collection {}, ID {}'.format(op['c'], match_doc_id))
            if assertion_type == "query-doc-style":
                print(
                    '  Query doc tested was:\n{}'.format(textwrap.indent(pprint.pformat(find_filter), '    ')))
            if existing_doc:
                print('  Existing doc is:\n{}'.format(textwrap.indent(pprint.pformat(existing_doc), '    ')))
            if 'i' in op:
                print('  Insert doc is:\n{}'.format(textwrap.indent(pprint.pformat(op['i']), '    ')))
            if 'u' in op:
                print('  Update doc is:\n{}'.format(textwrap.indent(pprint.pformat(op['u']), '    ')))
    print()


if __name__ == "__main__":
    main()
