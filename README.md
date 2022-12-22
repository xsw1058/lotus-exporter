# 使用说明

环境变量
```shell
export FULLNODE_API_INFO=<token>:/ip4/<ipaddress>/tcp/<port>/http
export MINERS_ID=<address1>:<tag1>,<address2>:<tag2>
export LOTUS_WALLETS=<address1>:<tag1>,<address2>:<tag2>
export GOLOG_LOG_LEVEL=info
export GOLOG_FILE=/<path>/exporter.log
```
> 日志参考：https://github.com/ipfs/go-log

编译并启动
```shell
go build main.go
sudo cp ./main  /usr/bin/lotus-export
sudo chmod +x /usr/bin/lotus-export
nohup /usr/bin/lotus-export &
```


# metrics


> base on:  https://github.com/s0nik42/lotus-farcaster

已初步完成. 剩余待完善
1. 添加注释
2. 编写test文件
3. 兼容lotus-farcaster的修改
4. grafana的优化

```shell

export GOLOG_LOG_LEVEL="debug,rpc=info"
export GOLOG_FILE=/ipfsdata/mainnet/exporter.log
export FULLNODE_API_INFO=<louts auth api-info>
export MINERS_ID=f097386:南1,f0134682:南2,f0149765:南3,f0494841:南4,f01070558:厦门,f0134006:北1,f0159632:北2,f01098119:福州,f01270285:泉州,f1gplsvrck3glszjga24l4ngvk4jz5usl5hkvj7mi:贾,f12wtveyh7pjixk7lfhwn2nf4j7bxysskfcbtk6ua:陈
```