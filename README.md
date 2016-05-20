# An Elasticsearch Migration Tool

Support cross version and http basic auth.

[![asciicast](https://asciinema.org/a/e562wy1ro30yboznkj5f539md.png)](https://asciinema.org/a/e562wy1ro30yboznkj5f539md)


## EXAMPLE:

copy index `index_name` from `192.168.1.x` to `192.168.1.y:9200`

```
./bin/esmove  -s http://192.168.1.x:9200   -d http://192.168.1.y:9200 -x index_name  -w=5 -b=10 -c 10000
```

copy index `src_index` from `192.168.1.x` to `192.168.1.y:9200` and save with `dest_index`

```
./bin/esmove -s http://localhost:9200 -d http://localhost:9200 -x src_index -y dest_index -w=5 -b=100
```

support Basic-Auth
```
./bin/esmove -s http://localhost:9200/ -x "src_index" -y "dest_index-test"  -d http://localhost:9201 -n admin:111111
```

## Download
https://github.com/medcl/elasticsearch-dump/releases


## Compile:

if download version is not fill you environment,you may try to compile it yourself. `go` required.

`make build`


## Options

```
  -s, --source=     source elasticsearch instance
  -d, --dest=       destination elasticsearch instance
  -m, --source_auth basic auth of source elasticsearch instance, ie: user:pass
  -n, --dest_auth   basic auth of target elasticsearch instance, ie: user:pass
  -c, --count=      number of documents at a time: ie "size" in the scroll request (10000)
  -t, --time=       scroll time (1m)
      --shards=     set a number of shards on newly created indexes
  -x, --src_indexes=    list of indexes to copy, comma separated (_all), support wildcard match(*)
  -y, --dest_index=    indexes name to save, allow only one indexname, original indexname will be used if not specified
  -a, --all         copy indexes starting with . and _ (false)
  -w, --workers=    concurrency (1)
  -b  --bulk_size 	bulk size in MB" default:5
  -v  --log 	    setting log level,options:trace,debug,info,warn,error
  -o  --dump_filepath dump to source index to local path
  -r  --dump_without_metadata don't include the metadata in the the dumpfile  default: false

```

Versions(Tested)
--------

From       | To
-----------|-----------
2.x | 2.x
2.x | 5.0
5.0 | 2.x
5.0 | 5.0

