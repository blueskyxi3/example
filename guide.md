# example 本地 external 运行指南

TiTu Edge 使用本机 3000 端口，example 使用 3012 端口。按下面顺序操作。

## 1 打开 TiTu Edge

保持桌面应用运行，确认可以登录 <http://127.0.0.1:3000/>。

## 2 准备插件

在终端复制执行：

```bash
cd ~/work/example/web
npm ci
npm run build

cd ~/work/example
mkdir -p data
cp -n config.demo.yaml data/config.local-debug.yaml
if [ ! -f data/.env.local-debug ]; then
  (umask 077; printf 'EXAMPLE_HOST_SECRET=%s\n' "$(openssl rand -hex 32)" > data/.env.local-debug)
fi
```

配置和密钥文件已存在时会保留原内容，不会重新生成密钥。

## 3 启动插件

在同一个终端执行：

```bash
cd ~/work/example
set -a
source data/.env.local-debug
set +a
go run . -config data/config.local-debug.yaml
```

看到 `example service listening` 后，保持这个终端运行。如果提示 3012 被占用，先在旧插件的终端按 Ctrl+C。

## 4 查看通信密钥

另开一个终端执行：

```bash
cat ~/work/example/data/.env.local-debug
```

找到 `EXAMPLE_HOST_SECRET=...`，复制**等号后面的值**。文件绝对路径是 `/Users/zouw/work/example/data/.env.local-debug`；其他用户替换为自己的目录。

## 5 在宿主连接插件

进入 TiTu Edge 的“插件市场”，找到“插件示例”，选择安装或试用。

如果市场里没有“插件示例”，先进入“插件管理 → 插件目录 → 登记插件”，粘贴本项目 [catalog.example.json](catalog.example.json) 的完整内容并保存。

安装时填写：

| 字段 | 填写内容 |
| --- | --- |
| 安装模式 | 外部服务 external |
| 服务 URL | `http://127.0.0.1:3012` |
| 控制面回调 | `http://127.0.0.1:3000` |
| 数据面回调 | `http://127.0.0.1:3000` |
| 通信密钥 Secret | 第 4 步复制的值 |

**两个回调地址都不加 `/v1`。** 已安装 external 时可直接检查运行信息；已有 process 安装时，需要先记录原配置，再卸载后重新安装为 external。

## 6 打开插件页面

刷新宿主页面，打开 <http://127.0.0.1:3000/plugins/example>。能看到调用记录页面，即说明页面接入成功；还没有调用时列表为空。

需要检查插件是否启动成功，可以执行：

```bash
curl http://127.0.0.1:3012/healthz
```

返回 `{"status":"ok"}` 即表示插件健康。

## 修改代码后

- 修改 Go 代码或规则：Ctrl+C 停止插件，再执行第 3 步。
- 修改前端：先在 `web` 目录执行 `npm run build`，再执行第 3 步并刷新整个宿主页面。
- 已安装 Delve 时，可以把第 3 步最后一行换成 `dlv debug . -- -config data/config.local-debug.yaml`，设置断点后执行 `continue`。

本指南先跑通本地页面接入。当前单机 master 的 external 模式不执行模型请求拦截；拦截逻辑可通过直接 SDK 请求调试，真实网关联调需要配置数据节点。
