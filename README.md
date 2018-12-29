# bwmon
A simple daemon to poll fast.com's bandwidth testing service and store
the results in influxdb.
 
## build
`go build .`

## run
`./bwmon -u "http://my-influxdb-host:8086/write?db=mydb"`

You should probably set this up under some process supervision tooling.
If you have auth setup, you need to add the proper query parameters to
the url given to the `-u` flag.

As of now, this tool stores the `min`, `max`, `mean`, and `stddev`
fields for each round of testing. Tags written include the short
hostname, app name, and app version.

See `./bwmon -h` for more options.

## meta
Thanks to ddo for: https://github.com/ddo/go-fast
