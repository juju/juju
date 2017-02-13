from __future__ import print_function

import argparse
import shutil
import ssl
import sys
import threading


try:
    import queue
    from urllib.request import build_opener, HTTPSHandler, ProxyHandler
except ImportError:
    import Queue as queue
    from urllib2 import build_opener, HTTPSHandler, ProxyHandler


def main(args):
    ctx = ssl.create_default_context()
    if args.insecure:
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE

    q = queue.Queue()
    for u in args.urls:
        t = threading.Thread(target=try_download, args=(u, ctx, args.noproxy, q))
        t.daemon = True
        t.start()

    failed = []
    for u in args.urls:
        u, result = q.get()
        if isinstance(result, Exception):
            failed.append((u, result))
            continue
        print('downloading {} to {}'.format(u, args.output), file=sys.stderr)
        with open(args.output, 'wb') as fout:
            shutil.copyfileobj(result, fout)
            result.close()
        sys.exit(0)

    for (u, exception) in failed:
        print('failed to download {}: {}'.format(u, exception), file=sys.stderr)
    sys.exit(1)


def try_download(url, ctx, noproxy, q):
    try:
        handlers = []
        if noproxy:
            handlers.append(ProxyHandler({}))
        handlers.append(HTTPSHandler(context=ctx))
        opener = build_opener(*handlers)
        resp = opener.open(url)
        q.put((url, resp))
    except Exception as e:
        q.put((url, e))


def arg_parser():
    parser = argparse.ArgumentParser(formatter_class=argparse.ArgumentDefaultsHelpFormatter)
    parser.add_argument('-o', dest="output", help="path to store file", type=str, required=True)
    parser.add_argument('--insecure', dest="insecure", help="disable cert validation and host checking", action="store_true", default=False)
    parser.add_argument('--noproxy', dest="noproxy", help="disable proxying of requests", action="store_true", default=False)
    parser.add_argument('urls', nargs='+')
    return parser


if __name__ == '__main__':
    main(arg_parser().parse_args())
