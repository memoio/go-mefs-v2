#/bin/bash

dir=$1/memo-go-contracts-v2
if [ -d "$dir" ]; then
    cd $dir
    git fetch origin master
else
    cd $1
    git clone git@132.232.87.203:508dev/memo-go-contracts-v2.git --depth=1
fi

dir=$1/memov2-contractsv2
if [ -d "$dir" ]; then
    cd $dir
    git fetch origin master
else
    cd $1 
    git clone git@132.232.87.203:508dev/memov2-contractsv2.git --depth=1
fi 

dir=$1/relay
if [ -d "$dir" ]; then
    cd $dir
    git fetch origin master
else
    cd $1
    git clone git@132.232.87.203:508dev/relay.git --depth=1
fi