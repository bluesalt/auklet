This post will attempt to introduce the design of Swift's replication engine in order to explain LOSF issue in Swift.

### Write
Let's begin with our hello world example.

```
curl -v -X PUT -H "Content-Type: text/plain" -d "Hello World" http://127.0.0.1:8080/v1/iqiyi/auklet/hi
```

This command creates a text object with the content `hello world`. We already know that the request will be send to proxy server firstly but the content will be saved by object server finally. Assume that there are 3 replicas per object. 

When proxy server receives the request, it will open 3 connections to the right object servers and send the object to them. we'll detail how Swift find the righ object servers in another post. 

Replication Engine saves every object as a standalone file in file system, no matter how big the size. So what is the file path of the object file? 

* Hash the object id `/iqiyi/auklet/hi` to parition, assume the hash value is `222`, then the partition is 222.
* Calculate the MD5 of `$PREFIX/iqiyi/auklet/hi$SUFFIX`, Both `PREFIX` and `SUFFIX` are some kind of configurable salt from `/etc/swift/swift.conf`. Assume we get the value `de0c277c7d5979e6440db018ebc66423`.

Now we can figure out the path of the object file which should be something like `/srv/node/vdd/objects/222/423/de0c277c7d5979e6440db018ebc66423/1529574899.69179.data`.  `/srv/node` is the so-called drive root and `/srv/node/vdb` is the mount point of disk `/dev/vdb`. `222` is the partition and `423` is the suffix of MD5. At last, `1529574899.69179` is the timestamp, the value of `X-Timestamp` header.

It is worth noting that Swift support [meta data management](https://docs.openstack.org/swift/latest/development_middleware.html#swift-metadata), and the meta data are saved as extended attributes of the object file. That is why Swift requires file systems that must support extended attributes.

### Read
When we try to download the object by 

```
curl -v  http://127.0.0.1:8080/v1/iqiyi/auklet/hi
```

Of course the request will be handled by proxy server firstly, and then it will send the requst to one of the object server. Assume that 'X-Newest' is not used. When the object server begins to process the request, it figures out the path of the object with the same route described in previous section.

If the object file exists, then it file content will be read and send back to user. Otherwise, 404 will be returned.

### Replicator
Everyone knows [CAP](https://en.wikipedia.org/wiki/CAP_theorem) per which Swift is a AP system and it adopts quorum write model. 2 success writes out of 3 replicas would be considered as success. That means it could be possible that 3 replicas are inconsistent at some point. So how do they achieve consistency eventually? The answer is the `object replicator` who is running as a background daemon in every object server. Its job is quite easy to understand.

* Check every local replica by traversing the disks.
* Check if the replica is consistent with the other 2 remote replicas.
    * If yes, nothing to do.
    * If not, check which replica is newer by the `X-Timestamp` value.
        * If any remote one is outdated, then it pushes local one to it.
        * Otherwise it do nothing. So replicator in push style. In this case, this local replica will get updated by remote replicators finally.

So how does replicator knows if replicas are consistent? Let's look back the object path `/srv/node/vdd/objects/222/423/de0c277c7d5979e6440db018ebc66423/1529574899.69179.data`. At first, replicator generates a dictionary for partition being replicated. Each key is the hash suffix like `423` and value is the MD5 of file name like `1529574899.69179.data`. After that, it pulls the countpart from remote hosts. For now, it could be easy to detect inconsistency state by comparing the entries.

Since the key is the hash suffix, each dictionary will have at most 4096 entries. So it is possible that a key could be shared by multiple objects. In that case, the value is the MD5 of the concation of filenames in order. At this time, we are not able to know which one is inconsistent exactly. So replicator will have to check each object under that suffix.

Everytime the dictionary is generated, replicator would have to list all the objects under the partition which could be time cost when there are tons of object in the file system. So under each partition directory, there is a `hashes.pkl` file which caches the whole dictionary. At some point in future, the dictionary will be regenerated. Besides `hashes.pkl`, another file called `hashes.invalid` could be found in partition directory. Since any write(PUT/POST/DELETE) to the parition causes change to the dictionary, in order not to rewrite `hashes.pkl` for every write request, it will append the suffix in `hashes.invalid`. During regeneration, both `hashes.pkl` and `hashes.invalid` will be consolidated.

### Auditor
Auditor amis at preventing silent data corruption issue. Like replicator, it traverses the disks and checks every objects with following actions.

* Check if meta data could be read properly
* Read out the whole object to calculate the MD5 to see if it matches with the metadata.

Any error detected would cause the object to be quarantined and thus inaccessable. Healthy replica would be rewritten by remote replicators laterly.

Provided that `/srv/node/vdd/objects/222/423/de0c277c7d5979e6440db018ebc66423/1529574899.69179.data` is corrupted, then it would be moved to `/srv/node/vdd/quarantined/objects/de0c277c7d5979e6440db018ebc66423/1529574899.69179.data`. It is worth mentioning that Swift will never delete the quarantined file mainly for the reason that a corrupted file means some disk sectors have become bad and it would be occupied by other files if the disk space is released. Thus it is the administrator's responsibility to handle the quarantined files.

Auditing objects requires high IOPS and may impact the normal read/write requests so Swift provides 2 options to avoid that. `files_per_second` limits maximum files audited per second per auditor process while `bytes_per_second` limits the maximum bytes per second. If the there are hundreds of thousands objects and `files_per_second` is set to a small value like 20, it may take years to audit all the objectst.

### Conclusion
Now let's summary why Swift is inadequate with small objects.

* Every request(CRUD) requires a multi-level file system search. And it is impossible to cache the hierarchy for each object which makes the seek time significant for small object.
* Backgroup services such as auditor and replicator have to traverse whole file system and could impact the performance of normal requests.
