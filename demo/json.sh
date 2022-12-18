#!/bin/sh
set -e
export TMPDIR=tmp
mkdir -p tmp
rm -rf tmp/*

rm -f json.log
perl ./json.pl json.log 10
../mackerel-plugin-axslog --key-prefix json --logfile json.log --ptime-key=reqtime --format json
perl ./json.pl json.log 1200000
#sleep 1\echo "--------------------"

ls -lh json.log
time ../mackerel-plugin-axslog --key-prefix json --logfile json.log --ptime-key=reqtime --format json ---filter example

echo "--------------------"


rm -f demo.log
perl ./demo.pl demo.log 10
../mackerel-plugin-axslog --key-prefix demo --logfile demo.log --ptime-key=reqtime
perl ./demo.pl demo.log 1200000
# sleep 1
echo "--------------------"

ls -lh demo.log
time ../mackerel-plugin-axslog --key-prefix demo --logfile demo.log --ptime-key=reqtime ---filter example
