# 使用说明
```shell
go build main.go
sudo cp ./main  /usr/bin/lotus-export
sudo chmod +x /usr/bin/lotus-export
```

环境变量
```shell
export FULLNODE_API_INFO=<token>:/ip4/<ipaddress>/tcp/<port>/http
export MINERS_ID=<address1>:<tag1>,<address2>:<tag2>
export LOTUS_WALLETS=<address1>:<tag1>,<address2>:<tag2>
export GOLOG_LOG_LEVEL=info
export GOLOG_FILE=/<path>/exporter.log
```
日志参考：https://github.com/ipfs/go-log