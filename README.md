# kklog

通过爬取效能平台 websocket 消息查看集群容器日志的一款小工具

## Usage

1. 准备工作

- 检查环境变量：`echo $HOME`
- 准备配置文件
  - 位置：`$HOME/.kkconf.yaml`
  - 示例内容：

    ```yaml
    user:
        name: kk
        token: xxx.yyy.zzz

        envs:
        -
            alias: wk_tag_manage
            deployment: wk-tag-manage
            name: wk-tag-manage
            type: api
            namespace: dev1
        -
            alias: tag-record-subscriber
            deployment: wk-tag-manage
            name: wk-tag-manage-tag-record-subscriber
            type: script
            namespace: dev1
    ```

2. 工具安装

```sh
make && make install
```

3. 使用

- 帮助信息

    ```sh
    kklog -h
    ```

- 查看日志

    ```sh
    kklog [配置文件中的别名]
    ```

    或者

    ```sh
    kklog -alias [配置文件中的别名]
    ```
