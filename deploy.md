# go-mefs-v2 deploy

go-mefs deploy 

## system prepare

准备事项：
1. deploy erc, pledge, role and fs contarct on settle chain  
2. create new group with security level(1 or at least 4)

## 角色初始化 

账户创建

1. init，初始化
2. 设置config.endPoint和roleContract 
3. charge (eth and erc20)，给此账户充值，由管理方操作
4. register， 注册账户

### keeper

1. pledge 质押
2. register as a keeper， 注册成为keeper角色
3. add to group (by admin)，加入某个组，由管理方操作
4. start，启动

### provider

1. pledge 
2. register as a provider
3. add to group
4. start

### user
 
1. add to group
2. start


## 查看信息

```
mefs-user info
```