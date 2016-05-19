# Elasticsearch Migrate Tool
//forked from: https://github.com/hoffoo/elasticsearch-dump/

## EXAMPLE:

copy index `index_name` from `192.168.1.x` to `192.168.1.y:9200`

```
./bin/esmove  -s http://192.168.1.x:9200   -d http://192.168.1.y:9200 -x index_name --docs-only  --time=2m -w=5 -b=10
```

copy index `src_index` from `192.168.1.x` to `192.168.1.y:9200` and save with `dest_index`

```
./bin/esmove -s http://localhost:9200 -d http://localhost:9200 -x src_index -y dest_index --docs-only --time=2m -w=5 -b=10
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
  -c, --count=      number of documents at a time: ie "size" in the scroll request (8000)
  -t, --time=       scroll time (1m)
  -f, --force       delete destination index before copying (false)
      --shards=     set a number of shards on newly created indexes
      --index-only  only create indexes, do not load documents (true)
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

