# 飞牛 HLS 跨进程验证

在工作树根目录执行：

```sh
bash plugins/fnmusic/scripts/hls-probe/run.sh linux
bash plugins/fnmusic/scripts/hls-probe/run.sh windows /mnt/c/Users/Public
```

单独检出飞牛模块时，可在该模块根目录运行 `bash scripts/hls-probe/run.sh linux`。

探针启动 `go-plugin` 子进程，通过 net/rpc 获取飞牛客户端解析的两段时长，用独立的 MuxBroker 数据流取回每段数据。环回 HTTP 夹具检查签名请求、从第二段开始取段、HTTP 410 后用原地址续期一次，以及两次关闭只向服务端发送一次释放请求。

另一组夹具在续期时返回新的播放清单地址，并让旧分段地址持续返回 HTTP 410。探针要求客户端读取新清单，用新地址取得原先请求的分段，随后也从新清单读取第一段。

断联用例启动独立探针宿主并让它跳过清理退出，验证子进程通过租约 EOF 释放飞牛服务端转码会话。播放器使用实际 MP3 夹具独立覆盖跨段定位和解码。真实设备行为仍需验收。
