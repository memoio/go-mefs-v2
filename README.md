# go-mefs-v2

go-mefs version2

## basic module

### repo

local repo


### auth

jsonrpc

### wallet


### network


### role

role manager

### txPool


### state


### node

basic node

## user

### lfs

manage user's data

### order

send data to provider


## provider

### order

receive data from user

### challenge

challenge-proof

## keeper

+ update challebeg epoch
+ confirm post income

## usage


### keeper

```
// compile
> make keeper
// init
> ./mefs-keeper init
// start; waiting for charge
> MEFS_PATH=$mpath ./mefs-keeper daemon --swarm-port=$port --api=$api --group=$gorupID 
// example
> ./mefs-keeper daemon --swarm-port=17201 --api=/ip4/127.0.0.1/tcp/18201 --group=2
```

### provider

```
// compile
> make provider
// init
> ./mefs-provider init
// start; waiting for charge
> MEFS_PATH=$mpath ./mefs-provider daemon --swarm-port=$port --api=$api --data-path=$dpath --group=$gorupID  
// example
> ./mefs-provider daemon --swarm-port=27201 --api=/ip4/127.0.0.1/tcp/28201 --data-path=/mnt --group=2 
```

### user

```
// compile
> make user
// init
> ./mefs-user init
// start; waiting for charge
> MEFS_PATH=$mpath ./mefs-user daemon --swarm-port=$port --api=$api --group=$gorupID
// example
> ./mefs-user daemon --swarm-port=37201 --api=/ip4/127.0.0.1/tcp/38201 --group=2
```
