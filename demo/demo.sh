#!/bin/sh
set -e
export TMPDIR=tmp
mkdir -p tmp
rm -rf tmp/*
rm -f demo.log
perl ./demo.pl demo.log 10
../mackerel-plugin-axslog --key-prefix demo --logfile demo.log --ptime-key=reqtime
perl ./demo.pl demo.log 1200000
ls -lh demo.log
time ../mackerel-plugin-axslog --key-prefix demo --logfile demo.log --ptime-key=reqtime

echo "--------------------"

rm -f demo.log
perl ./demo.pl demo.log 10
./mackerel-plugin-accesslog --format ltsv demo.log
perl ./demo.pl demo.log 1200000
ls -lh demo.log
time ./mackerel-plugin-accesslog --format ltsv demo.log
