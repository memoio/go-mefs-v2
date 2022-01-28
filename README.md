# go-mefs-v2

go-mefs v2 

## basic module

### repo

本地目录管理：配置文件


### auth

jsonrpc授权

### wallet


### network

网络发送和接收

### role

角色管理，从结算层获取的结果


### txPool

数据交易池

### state

数据状态


### node

包含所有基础功能

## user

### lfs

管理用户数据空间

### order

数据发送管理


## provider

### order

数据接收管理

### challenge

proof管理

## keeper

更新chalEpoch
确认post income


## usage


### keeper

```
// compile
> make keeper
// init
> ./mefs-keeper init
// start
> MEFS_PATH=$mpath ./mefs-keeper daemon --swarm-port=$port --api=$api --group=$gorupID 
// example
> /mefs-keeper daemon --swarm-port=27201 --api=/ip4/127.0.0.1/tcp/28201 --group=2
```

### provider

```
// compile
> make provider
// init
> ./mefs-provider init
// start
> MEFS_PATH=$mpath ./mefs-provider daemon --swarm-port=$port --api=$api --data-path=$dpath --group=$gorupID  
```
