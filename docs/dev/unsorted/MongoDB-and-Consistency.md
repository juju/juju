(mongodb-consistency)=
# MongoDB consistency
<!-- TODO(gfouillet): do not merge into 4.0, or delete whenever merged (reason: related to mongodb) -->

The `state` package (and some others) deals with persisting our model to MongoDB. It uses `gopkg.in/mgo.v2`, and
specifically the `txn` subpackage, to implement consistency. The transaction model is an elegant generalisation of
two-phase commit, but it is *not your friend*. In particular, the transactions are:

* *Not atomic*: if you want to be sure that every transaction actually completes, you need to run `ResumeAll()` at
  regular intervals. So long as you do that, every transaction will *eventually* either completely apply or not apply at
  all.

* *Not consistent*: unless you take great care to ensure that *every* operation in *every* transaction you construct is
  itself consistent.

* *Not isolated*: so you *will* get dirty reads *all the time*. And even if you roll your own flawless consistency for
  writes (which you should) the read patterns can be such that a set of documents read into memory at the same time may
  *still* not actually be consistent.

* *As durable as the undead*: If your process goes down at the right moment, a transaction can be queued but not
  processed, and not picked up until a `ResumeAll()` which you should generally assume is coming arbitrarily far in the
  future. So you can ask for a change; see a failure; and then 5 minutes later have the requested change written to the
  database.

I hope it is clear that it is extremely important to read and understand
the [documentation](http://godoc.org/labix.org/v2/mgo/txn)
and [explanatory blog post](http://blog.labix.org/2012/08/22/multi-doc-transactions-for-mongodb). Most people are not
familiar with this sort of model, and that's fine; and it can help to talk it through as you're coming up to speed, and
fwereade will gladly help if/when you get lost.

But if you try to write this code without properly orienting yourself first you *will* screw something up, and the
consequences of small mistakes can be *catastrophic*. A few particular notes that maybe don't obviously follow from the
above:

* Once mgo/txn has touched a document you MUST NOT modify it out-of-band. There is important state stored *in your
  document*; it may pertain to arbitrarily distant documents; and the guarantees provided by mgo/txn depend upon it

* Even via mgo/txn be *especially* careful of the "txn-revno" and "txn-queue" fields, which store the aforementioned
  state. There is sometimes justification for reading or asserting a txn-revno; and there is *very* occasionally
  justification for declaring a struct field that serializes as such; but if you do that you must be *certain* never to
  use that doc type with $set.

* *Overlapping* transactions are (while admittedly necessary, and kinda the point of the whole endeavour) risky.
  Transactions that touch the same documents cannot execute in parallel; this is fine in the small, but trying to
  execute N overlapping transactions has a worst-case of *at least* O(N^2) time *and* space. (Worst-case per flusher is
  at least linear with the size of the set of transitively connected documents; flusher count is linear with transaction
  count.)

* In particular, some parts of the model represent the user's desires (which we are trying to realise) and some parts
  represent aspects of reality (which we need to operate on or communicate back to the user). The write patterns for
  these kinds of information are very different; putting them in the same document is asking for O(N^2)-flavoured
  trouble (*and* is inconvenient for the watcher system, which can only operate at document granularity).

* That previous point applies at every level of scale. If you're just dumping fields into documents as you would treat a
  stateful OO type, you're laying traps for yourself and others; the default approach should be to create new documents
  for data with *either* new write patterns *or* new read patterns.

* It is also important to note that it *is* possible to build a reliable system on top of these primitives. But if you
  try to treat mgo/txn like postgres, it will cut you.

There's an [example here](mgo-txn-example.md).