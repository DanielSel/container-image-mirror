#!/usr/bin/env bash
LDIR=$(dirname "$0")
TEST=$1
if [ ! -z "$2" ]
then
    DIR=$2
else
    DIR=$LDIR
fi

go test -timeout 120m -v -cpuprofile "${DIR}/results_${TEST}_cpu.pprof" -memprofile "${DIR}/results_${TEST}_mem.pprof" "${LDIR}/../test/${TEST}"
