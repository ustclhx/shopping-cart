#!/bin/bash 
go build benchmark/stress.go
if [[ $? == 0 ]]; then ./stress -d -c 200 -d 10000;fi
