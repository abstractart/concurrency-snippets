#!/bin/bash

versions=(2.6.10 3.4.1)

for version in ${versions[*]}
do
    rbenv local $version
    ruby -v

    ruby $1
    echo ""
done