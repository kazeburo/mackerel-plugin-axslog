# mackerel-plugin-axslog

mackerel-plugin-accesslog alternative with great perfomance.
axslog supports JSON and LTSV formated log and allows to change response time and status label in log.

blog Entry(in japanese): https://kazeburo.hatenablog.com/entry/2019/04/05/093000


## Usage

```
% ./mackerel-plugin-axslog -h
Usage:
  mackerel-plugin-axslog [OPTIONS]

Application Options:
      --logfile=    path to nginx ltsv logfiles. multiple log files can be specified, separated by commas.
      --format=     format of logfile. support json and ltsv (default: ltsv)
      --key-prefix= Metric key prefix
      --ptime-key=  key name for request_time (default: ptime)
      --status-key= key name for response status (default: status)
      --filter=     text for filtering log
  -v, --version     Show version

Help Options:
  -h, --help        Show this help message
```

## Install

Please download release page or `mkr plugin install kazeburo/mackerel-plugin-axslog`.