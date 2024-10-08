# Query Profiling

<!-- TODO(gfouillet): do not merge into 4.0, or delete whenever merged (reason: related to mongodb) -->

By default MongoDB's profiler will record queries which took more than 100ms. You can configure this threshold as well
turning profiling on and off using the db.setProfilingLevel command.

See here: https://docs.mongodb.com/manual/reference/method/db.setProfilingLevel/

As well as logging slow queries the profiler also records them to the db.system.profile collection along with a bunch of
stats about them. This let's you do interesting queries on the recorded queries.

Be aware that the profiler incurs a performance cost. It's probably not a good idea to leave it on all the time,
especially at lower thresholds.

Examples:

Getting the mongo shell going:

```
user=$(basename `ls -d /var/lib/juju/agents/machine-*`)
password=`sudo grep statepassword /var/lib/juju/agents/machine-*/agent.conf  | cut -d' ' -f2`
/usr/lib/juju/mongo*/bin/mongo 127.0.0.1:37017/juju  --authenticationDatabase admin --ssl --sslAllowInvalidCertificates --username "$user" --password "$password"
```

Queries that took more than 50ms:

```
db.system.profile.find({millis: {$gt: 50}})
```

Top 10 collections involved with recorded queries in the last 60s:
(just to show what's possible)

```
db.system.profile.aggregate([
  { $match: {ts: { $gt: new Date(new Date() - (60 * 1000))}} },
  { $group: {_id: "$ns", count: {$sum: 1}} },
  { $sort: {count: -1} },
  { $limit: 10 }
])
```

# mongotop

mongotop shows per collection stats at regular intervals. It's useful for find the busiest collections across all
databases. This should be quite useful.

The base commmand for use on a Juju controller is:

```
# user and password as above
/usr/lib/juju/mongo*/bin/mongotop --host 127.0.0.1:37017  --authenticationDatabase admin --ssl --sslAllowInvalidCertificates --username "$user" --password "$password" <interval in seconds>
```

Get the username and password from the agent.conf as usual. The interval is optional.

# mongostat

mongostat periodically reports various stats from the mongod server. It may help to point to an issue.

The base command for use on a Juju controller is:

```
# user and password as above
/usr/lib/juju/mongo*/bin/mongostat --host 127.0.0.1:37017  --authenticationDatabase admin --ssl --sslAllowInvalidCertificates --username "$user" --password "$password" <interval in seconds>
```

The meaning of the output fields is documented here: https://docs.mongodb.com/manual/reference/program/mongostat/#fields

With MongoDB 3.4 you can add more fields but not with 3.2.

# Observing Current Activity

If you suspect the MongoDB server may be taking a long time on a particular activity the `db.currentOp()` command will
show you what the server is currently up to.

# Collection Stats

A large amount of useful statistics about a particular collection can be obtained like this in the mongo shell:

```
db.<collection>.stats()
```




