# go-mefs-v2

go-mefs version2

## basic module

### repo

local repo: contains config.json, token file, and data, dht, keystore, meta, state directory

### auth

jsonrpc: used to interact with core service

### wallet

use password to protect keystore file

### network

manage network tranmission

### role

role manager

### txPool

manage tx message and sync

### state

maintain status of data chain

### node

basic node

## user service

### lfs

manage user's data

### order

send data to provider

## provider

### order

receive data from user

### challenge

challenge local data per epoch

## keeper

+ update challenge epoch
+ confirm post income
+ submit subOrder when order is expired

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

### info

```
> ./mefs-user info

----------- Information -----------
2022-03-21 11:14:57.282231091 +0800 CST m=+0.062574412
----------- Network Information -----------
ID:  12D3KooWGfz9746oyBKWMKcQqkhnofSF9J3DF12JAdi2RBbw6N64
IP:  [/ip4/192.168.1.46/tcp/32215]
Type: Private
----------- Sync Information -----------
Status: true, Slot: 317189, Time: 2022-03-21 11:14:30 CST
Height Synced: 8607, Remote: 8607
Challenge Epoch: 39 2022-03-21 10:59:30 CST
----------- Role Information -----------
ID:  114
Type:  User
Wallet:  0xa4BdB1F76a48c8e7cf1425c471aBE3e5C216dcA8
Balance: 993.83 Gwei (tx fee), 0 AttoMemo (Erc20), 1.56 Memo (in fs)
Data Stored: size 5371084800 byte (5.00 GiB), price 5287500000
----------- Group Information -----------
EndPoint:  https://devchain.metamemo.one:8501
Contract Address:  0x15DB6043DFC4eAE279957D0C682dDbFCd529f3fb
Fs Address:  0xEd8c550F2511bcDD23437b69c6Be71C3e47A4633
ID:  1
Security Level:  7
Size:  5.44 GiB
Price:  479923788000
Keepers: 10, Providers: 99, Users: 9
----------- Pledge Information ----------
Pledge: 0 AttoMemo, 109.00 Memo (total pledge), 109.03 Memo (total in pool)
----------- Lfs Information ----------
Status:  true
Buckets:  1
Used: 5.00 GiB
Raw Size: 5.00 GiB
Confirmed Size: 5.00 GiB
OnChain Size: 5.00 GiB
Need Pay: 46062754.20 NanoMemo
Paid: 46062754.20 NanoMemo
```

### lfs ops

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
```


### order ops

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
```

