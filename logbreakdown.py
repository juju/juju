from datetime import datetime, timedelta
import gzip

LOG_BREAKDOWN_SECONDS = 20
dt_format = '%Y-%m-%d %H:%M:%S'


def main():
    # used for initial testing.
    timestamps = [
        ('2016-07-21 08:33:12', '2016-07-21 08:38:15'),
        ('2016-07-21 08:39:46', '2016-07-21 08:42:42'),
        ('2016-07-21 08:42:43', '2016-07-21 08:43:14'),
    ]
    timestamps = [
        ('2016-07-20 22:40:52', '2016-07-20 22:44:48'),  # Bootstrap
        ('2016-07-20 22:46:01', '2016-07-20 22:57:13')   # Deploy
    ]

    breakdown_log_by_timeframes('./machine-0.log.gz', timestamps)


def breakdown_log_by_timeframes(log_file, timestamps):
    all_log_breakdown = dict()
    for event_range in timestamps:
        range_breakdown = []
        range_start = datetime.strptime(event_range[0], dt_format)
        range_end = datetime.strptime(event_range[1], dt_format)
        range_name = '{} - {}'.format(range_start, range_end)

        next_step = range_start + timedelta(seconds=LOG_BREAKDOWN_SECONDS)

        if next_step > range_end:
            range_breakdown.append((range_start, range_end))
        else:
            while next_step < range_end:
                range_breakdown.append((range_start, next_step))

                range_start = next_step
                next_step = range_start + timedelta(
                    seconds=LOG_BREAKDOWN_SECONDS)
                # Otherwise there will be overlap.
                range_start += timedelta(seconds=1)

                if next_step >= range_end:
                    range_breakdown.append((range_start, range_end))
        breakdown = get_timerange_logs(log_file, range_breakdown)
        all_log_breakdown[range_name] = breakdown

        # for timeframe in sorted(breakdown.keys()):
        #     print(
        #         '{tf}:\n\t{first}\n\t..[skip {count}]..\n\t{last}'.format(
        #             tf=timeframe,
        #             first=breakdown[timeframe][0],
        #             count=len(breakdown[timeframe]) - 2,
        #             last=breakdown[timeframe][-1],
        #         )
        #     )
        # print('-'*80)
    return all_log_breakdown


def get_timerange_logs(log_file, timestamps):
    log_breakdown = dict()
    previous_line = None
    # with open('./machine-0.log', 'rt') as f:
    with gzip.open(log_file, 'rt') as f:
        for log_range in timestamps:
            range_start = log_range[0]
            range_end = log_range[1]
            log_lines = []
            range_str = '{} - {}'.format(range_start, range_end)

            if previous_line:
                if log_line_within_start_range(previous_line, range_start):
                    log_lines.append(previous_line)
                previous_line = None

            for line in f:
                if log_line_within_start_range(line, range_start):
                    break
            else:
                print('LOG: failed to find start')
                # continue?
                break

            # It it's out of range of the end range then there is nothing for
            # this time period.
            if not log_line_within_end_range(line, range_end):
                # do we actually need to add blank string? Perhaps just no
                # addition is fine.
                log_breakdown[range_str] = ['No log contents']
                previous_line = line
                continue

            log_lines.append(line)

            for line in f:
                if log_line_within_end_range(line, range_end):
                    log_lines.append(line)
                else:
                    previous_line = line
                    break
            log_breakdown[range_str] = log_lines

    return log_breakdown


def log_line_within_start_range(line, range_start):
    datestamp = " ".join(line.split()[0:2])
    try:
        ds = datetime.strptime(datestamp, dt_format)
    except ValueError:
        # Don't want an early entry point to the logging.
        return False

    if ds > range_start or ds == range_start:
        return True
    return False


def log_line_within_end_range(line, range_start):
    datestamp = " ".join(line.split()[0:2])
    try:
        ds = datetime.strptime(datestamp, dt_format)
    except ValueError:
        # Fine to collect this line as we haven't hit a dated line that is
        # after our target.
        return True

    if ds < range_start or ds == range_start:
        return True
    return False


if __name__ == '__main__':
    main()
