## juju-mongotop

The following parses a series of mongotop files and outputs a sorted list of
namespaced collections.

Takes a file containing many of the following:

```
                                            ns    total    read    write    2022-03-15T08:20:12Z
                                     juju.txns    319ms     0ms    319ms                        
                        local.replset.minvalid    241ms     0ms    241ms                        
                                local.oplog.rs    212ms    95ms    117ms                        
                               juju.unitstates    127ms     0ms    127ms                        
                                    juju.units    100ms     0ms    100ms                        
                                 juju.txns.log     79ms     0ms     79ms                        
                          juju.statuseshistory     61ms     0ms     61ms                        
logs.logs.d16df13e-f09d-4b03-816a-193c38b7d38c     58ms     0ms     58ms                        
logs.logs.4f55c931-cba6-4d18-8620-d72c9053949e     29ms     0ms     29ms                        
logs.logs.480368cd-73e4-4950-85b5-292c7ee64fe5     26ms     0ms     26ms                        
```

And outputs the following:

```
+----------------+-----------+-------+-----------+
|       NS       |   TOTAL   | READ  |   WRITE   |
+----------------+-----------+-------+-----------+
| local.oplog.rs | 1m20.921s | 965ms | 1m19.955s |
| local.oplog.rs | 1m5.055s  | 9ms   | 1m5.045s  |
| local.oplog.rs | 1m5.011s  | 23ms  | 1m4.987s  |
| local.oplog.rs | 1m4.257s  | 15ms  | 1m4.241s  |
| local.oplog.rs | 1m3.332s  | 9ms   | 1m3.322s  |
| local.oplog.rs | 1m2.645s  | 74ms  | 1m2.57s   |
| local.oplog.rs | 1m2.188s  | 12ms  | 1m2.175s  |
| local.oplog.rs | 1m1.608s  | 21ms  | 1m1.587s  |
| local.oplog.rs | 1m1.269s  | 38ms  | 1m1.23s   |
| local.oplog.rs | 1m1.138s  | 10ms  | 1m1.127s  |
+----------------+-----------+-------+-----------+
```