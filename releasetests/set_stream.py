#!/usr/bin/env python

from argparse import ArgumentParser
import json
import os

from simplestreams.generate_simplestreams import json_dump


def main():
    parser = ArgumentParser()
    parser.add_argument('in_file', metavar='IN-FILE',
                        help='The file to read.')
    parser.add_argument('out_file', metavar='OUT-FILE',
                        help='The file to write.')
    parser.add_argument(
        'stream_id', metavar="STREAM-ID",
        help='The new stream for the items.  By default, a revision-build'
        ' stream.')
    parser.add_argument(
        '--update-path', action='store_true',
        help='Update the path to put the agent in "agent/STREAM"')
    parser.add_argument(
        '--agent-stream', action='store_true',
        help='Interpret STREAM-ID as an agent-stream value, not a'
        ' revision build.')
    args = parser.parse_args()
    with open(args.in_file) as in_file:
        stanzas = json.load(in_file)
    if args.agent_stream:
        stream = args.stream_id
    else:
        stream = 'revision-build-{}'.format(args.stream_id)
    content_id = 'com.ubuntu.juju:{}:tools'.format(stream)
    for stanza in stanzas:
        stanza['content_id'] = content_id
        if not args.update_path:
            continue
        path = os.path.join(
            'agent', stream, os.path.basename(stanza['path']))
        stanza['path'] = path
    json_dump(stanzas, args.out_file)


if __name__ == '__main__':
    main()
