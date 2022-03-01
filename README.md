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

#### lfs ops

```
> ./mefs-user lfs

NAME:
   mefs-user lfs - Interact with lfs

USAGE:
   mefs-user lfs command [command options] [arguments...]

COMMANDS:
   createBucket  create bucket
   listBuckets   list buckets
   headBucket    head bucket info
   putObject     put object
   headObject    head object
   getObject     get object
   listObjects   list objects
   help, h       Shows a list of commands or help for one command
```shell

#### order ops

```
> ./mefs-user order
NAME:
   mefs-user order - Interact with order

USAGE:
   mefs-user order command [command options] [arguments...]

COMMANDS:
   jobList  list jobs of all pros
   payList  list pay infos all pros
   get      get order info of one provider
   detail   get detail order seq info of one provider
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help (default: false)
```shell

### info

```
> ./mefs-user info
----------- Information -----------
2022-03-01 09:58:32.683372776 +0800 CST m=+0.028990227
----------- Sync Information -----------
Status: true, Slot: 259436, Time: 2022-03-01 09:58:00 CST
Height Synced: 22069, Remote: 22070
Challenge Epoch: 123
----------- Role Information -----------
ID:  126
Type:  User
Wallet: 0x6baf0fa94d410cb22a9e1bde8605949110354d1d
Balance: 100009745.09 Gwei (on chain), 679.78 Token (Erc20), 165600934.22 Gwei (in fs)
Data Stored: size 548454293504 byte (510.79 GiB), price 548454293504000
----------- Group Information -----------
ID:  1
Security Level:  7
Size:  1.20 TiB
Price:  1316841684992000
Keepers: 10, Providers: 99, Users: 57
----------- Pledge Information ----------
Pledge: 0 Wei, 109.00 Token (total pledge), 329.40 Token (total in pool)
----------- Lfs Information ----------
Status:  true
Buckets:  3
Used: 527.91 GiB
Raw Size: 527.29 GiB
Confirmed Size: 527.46 GiB
OnChain Size: 527.29 GiB
Need Pay: 500.14 Token
Paid: 500.14 Token
```shell