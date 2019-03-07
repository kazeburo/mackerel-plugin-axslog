# mackerel-plugin-axslog

mackerel-plugin-axslog alternative. This allow to change response time and status label in log.
And also support JSON and LTSV formated log.


## Usage

```
Usage:
  mackerel-plugin-axslog [OPTIONS]

Application Options:
      --logfile=    path to nginx ltsv logfile
      --format=     format of logfile. support json and ltsv (ltsv)
      --key-prefix= Metric key prefix
      --ptime-key=  key name for request_time (ptime)
      --status-key= key name for response status (status)
  -v, --version     Show version

Help Options:
  -h, --help        Show this help message
```
