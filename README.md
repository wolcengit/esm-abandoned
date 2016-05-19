# Elasticsearch Migrate Tool
//forked from: https://github.com/hoffoo/elasticsearch-dump/

## EXAMPLE:

copy index `index_name` from `192.168.1.x` to `192.168.1.y:9200`

```
./bin/esmove  -s http://192.168.1.x:9200   -d http://192.168.1.y:9200 -x index_name  -w=5 -b=10 -c 10000
```

copy index `src_index` from `192.168.1.x` to `192.168.1.y:9200` and save with `dest_index`

```
./bin/esmove -s http://localhost:9200 -d http://localhost:9200 -x src_index -y dest_index -w=5 -b=100
```

desc es use basic auth
```
./bin/esmove -s http://localhost:9200/ -x "src_index" -y "dest_index-test"  -d http://localhost:9201 -n admin:111111
```

## Compile:

1. make build
2. make cross-build 

## Download
https://github.com/medcl/elasticsearch-dump/releases


## Options

```
  -s, --source=     source elasticsearch instance
  -d, --dest=       destination elasticsearch instance
  -m, --source-auth basic auth of source elasticsearch instance, eg: user:pass
  -n, --dest-auth   basic auth of target elasticsearch instance, eg: user:pass
  -c, --count=      number of documents at a time: ie "size" in the scroll request (10000)
  -t, --time=       scroll time (1m)
      --shards=     set a number of shards on newly created indexes
  -x, --src_indexes=    list of indexes to copy, comma separated (_all), support wildcard match(*)
  -y, --dest_index=    indexes name to save, allow only one indexname, original indexname will be used if not specified
  -a, --all         copy indexes starting with . and _ (false)
  -w, --workers=    concurrency (1)
  -b  bulk_size 	bulk size in MB" default:5
  -v  log 	setting log level,options:trace,debug,info,warn,error

```

Versions(Tested)
--------

From       | To
-----------|-----------
2.x | 2.x
2.x | 5.0
5.0 | 2.x
5.0 | 5.0

