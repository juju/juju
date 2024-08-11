#!/usr/bin/env python3
# Copyright 2019 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

import argparse
import re
import sys

def main(args):
    p = argparse.ArgumentParser(description="parse claim log files, reporting output")
    p.add_argument("file", type=argparse.FileType('r'), default=sys.stdin, nargs="?",
            help="the name of the file to parse")
    p.add_argument("--tick", type=float, default=1.0,
            help="seconds between printing status ticks")
    opts = p.parse_args(args)
    actionsRE = re.compile("\\s*(?P<time>\\d+\\.\\d\\d\\d)s\\s+(?P<action>claimed|extended|lost|connected).*in (?P<duration>[0-9m.]+s)")
    # We don't have minutes if we have 'ms', so match 'ms' first, we might have 'm' if we have 's', so put it before s.
    durationRE = re.compile("((?P<milliseconds>\\d+)ms)?((?P<minutes>\\d+)m)?((?P<seconds>\\d+(\\.\\d+)?)s)?")
    totalClaims = 0
    extendedSum = 0
    extendedCount = 0
    lostCount = 0
    lastTime = 0
    claimSum = 0
    claimCount = 0
    print("claims\tclaim time\textend time\tlost")
    for line in opts.file:
        m = actionsRE.match(line.strip())
        if m is None:
            continue
        curTime, action, duration = m.group('time', 'action', 'duration')
        curTime = float(curTime)
        m2 = durationRE.match(duration)
        if m2 is None:
            print("could not match %q" % (duration,))
            continue
        m, s, ms = m2.group("minutes", "seconds", "milliseconds")
        delta = 0
        if m is not None:
            delta += float(m)*60
        if s is not None:
            delta += float(s)
        if ms is not None:
            delta += float(ms) * 0.001
        delta = round(delta, 3)
        # print(action, duration, delta)
        if action == "extended":
            extendedCount += 1
            extendedSum += delta
        elif action == "lost":
            totalClaims -= 1
            lostCount += 1
        elif action == "claimed":
            totalClaims += 1
            claimCount += 1
            claimSum += delta
        if curTime - lastTime > opts.tick:
            lastTime = curTime
            claimMsg = ""
            if claimCount > 0:
                claimAvg = claimSum / claimCount
                claimCount = 0
                claimSum = 0
                claimMsg = "%9.3f" % (claimAvg,)
            extendedMsg = " "*9
            if extendedCount > 0:
                extendedAvg = extendedSum / extendedCount
                extendedSum = 0
                extendedCount = 0
                extendedMsg = "%9.3f" %(extendedAvg,)
            print("%5d\t%9s\t%9s\t%d" % (totalClaims, claimMsg, extendedMsg, lostCount))

if __name__ == "__main__":
    main(sys.argv[1:])

