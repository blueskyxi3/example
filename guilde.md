# example 插件本地运行与调试指南

本地 TiTu Edge 已提供 `http://localhost:3000/v1`。要在同一个桌面宿主中验证插件页面、请求拒绝和响应替换，使用“本机托管 process”安装 example。插件进程由桌面宿主启动，地址、端口和通信密钥由部署器配置。

适用环境：TiTu Edge 桌面宿主监听本机 3000 端口，example 与 ai-hub 仓库放在同级目录，默认位置分别为 `~/work/example` 和 `~/work/ai-hub`。宿主需要支持 SDK v2、插件市场和本机托管 process。

## 地址与模式

| 项目 | 本地值 |
| --- | --- |
| 宿主 origin 与插件回调地址 | `http://127.0.0.1:3000` |
| OpenAI 兼容 API Base URL | `http://127.0.0.1:3000/v1` |
| 请求完整路径 | `http://127.0.0.1:3000/v1/chat/completions` |
| 插件管理与目录登记 | `http://127.0.0.1:3000/panel/plugin-management` |
| 插件市场 | `http://127.0.0.1:3000/plugin-marketplace` |
| example 页面 | `http://127.0.0.1:3000/plugins/example` |

浏览器登录、安装和查看页面时统一使用 `127.0.0.1`。`localhost` 与 `127.0.0.1` 的浏览器 Cookie 不共用；切换后可能需要重新登录。

**插件的 `host_url` 和 `data_host_url` 填宿主 origin，不带 `/v1`。** 模型客户端使用的 API Base URL 才带 `/v1`。

当前源码按宿主角色选择 external 扩展点：单机 master 上 external 提供页面等控制面能力；process 则可以在同一个 master 上执行 example 声明的请求与响应拦截。因此，本指南用 process 完整验证本机 `/v1` 链路。

## 第一步 停止旧的手动插件进程

如果之前手动启动过 example，在对应运行终端按 Ctrl+C 停止该进程。TiTu Edge 桌面应用继续运行。

后续由插件市场启动 process，不需要再手动启动 3012 服务，也不用创建本地与 PoC 之间的隧道。

若 example 已经以 external 方式安装，在市场先记录原运行配置，再停用、卸载该安装，然后按以下步骤安装 process。仅更新目录中的 `modes` 不会改变已安装插件的部署模式。卸载后保留的数据和授权按市场实际状态复用。

## 第二步 构建带演示规则的本地调试包

在终端执行以下整段命令。生成文件写入 example 的 `dist`，不修改项目的配置模板和 Manifest：

```bash
cd "$HOME/work/example/web"
npm ci
npm run build -- --sourcemap

cd "$HOME/work/example"
mkdir -p dist data
go build -gcflags='all=-N -l' -o dist/example .

python3 - <<'PY'
import json
import pathlib
import subprocess
import zipfile

root = pathlib.Path.cwd()
manifest = json.loads((root / 'plugin.json').read_text())
platform = '-'.join(subprocess.check_output(['go', 'env', 'GOOS', 'GOARCH'], text=True).split())
package = root / 'dist' / f"example-{manifest['version']}-{platform}-local-debug.zip"
with zipfile.ZipFile(package, 'w', zipfile.ZIP_DEFLATED) as archive:
    archive.write(root / 'dist/example', 'example')
    archive.write(root / 'plugin.json', 'plugin.json')
    archive.write(root / 'config.demo.yaml', 'config.yaml')

catalog = {
    'manifest': manifest,
    'description': '本地调试：请求拒绝、替换、响应脱敏和调用记录',
    'modes': ['process'],
    'artifacts': {platform: str(package)},
    'host_url': 'http://127.0.0.1:3000',
    'data_host_url': 'http://127.0.0.1:3000',
    'env_prefix': 'EXAMPLE',
    'trial_days': 7,
    'purchase_days': 0,
    'rollback_compatible': False,
}
target = root / 'dist/catalog.local-debug.json'
target.write_text(json.dumps(catalog, ensure_ascii=False, indent=2) + '\n')
print('安装包:', package)
print('目录记录:', target)
print('SQLite 文件路径:', root / 'data/example-local-process.db')
PY
```

打包时把 `config.demo.yaml` 保存为 ZIP 根目录中的 `config.yaml`，确保首次安装就带有：

```yaml
rules:
  request_blocked: ["EXAMPLE_BLOCK"]
  request_replacements:
    - from: "EXAMPLE_PRIVATE"
      to: "[private]"
  response_replacements:
    - from: "EXAMPLE_SECRET"
      to: "[redacted]"
```

项目默认 `make build` 打包的是空规则 `config.example.yaml`，所以完整演示使用上面的调试包。`-gcflags` 保留适合断点调试的编译结果；source map 用于浏览器定位前端源码。

## 第三步 登记本地插件目录

1. 在本地桌面环境使用有插件管理权限的管理员登录。
2. 打开左侧“插件管理”，选“插件目录”。
3. 点击“登记插件”。已有 example 目录项时编辑该项。
4. 将 `~/work/example/dist/catalog.local-debug.json` 的完整内容粘贴到 JSON 编辑框，保存。

这里填写的是目录记录，不是 `plugin.json`，也不是 `registration.example.json`。

生成记录的 `artifacts` 会自动匹配本机平台，指向本机 ZIP 的绝对路径。宿主与插件构建都在本机，无需上传 GitHub Release。目录版本来自本地 Manifest，首次安装时应与包内 Manifest 一致。

## 第四步 在市场安装本机托管插件

在桌面左侧快捷栏点击拼图图标进入“插件市场”，或打开 `http://127.0.0.1:3000/plugin-marketplace`。

找到“插件示例”，按已有授权选择“安装”、试用或购买并安装。安装参数：

| 字段 | 填写值 |
| --- | --- |
| 安装模式 | 本机托管 process |
| 控制面回调 | `http://127.0.0.1:3000` |
| 数据面回调 | `http://127.0.0.1:3000` |
| 共用主项目数据库 | 取消勾选 |
| 独立数据库类型 | SQLite |
| SQLite 文件绝对路径 | 构建命令输出的 SQLite 文件绝对路径 |
| 其他配置 | 保持表单默认值 |

process 的端口和通信密钥由宿主生成，无需填写之前的 `EXAMPLE_HOST_SECRET`。安装后点击插件卡片，查看运行信息，确认 mode 为 process、状态为运行中，记录 PID 和实际插件地址。端口不一定是 3012。

如果提示没有 process 安装能力，检查桌面版本和内嵌部署器状态；TiTu Edge 桌面包按设计支持本机托管，无需 Docker。

## 第五步 打开页面并发送模型请求

刷新宿主页面，打开 `http://127.0.0.1:3000/plugins/example`。没有执行记录时显示空列表属于正常情况。

在“渠道管理”中准备可用文本模型渠道，确认当前用户分组、模型权限和价格已配置。在“令牌管理”中复制一把用于测试的 API Key，设置当前终端变量：

```bash
export HUB_API_KEY='粘贴本地令牌管理中复制的API Key'
export MODEL='填写本地已配置并允许该令牌使用的模型ID'

curl --fail http://127.0.0.1:3000/v1/models \
  -H "Authorization: Bearer $HUB_API_KEY"
```

先发送普通请求，确认渠道能响应。然后发送拒绝请求：

```bash
python3 -c 'import json,os; print(json.dumps({"model":os.environ["MODEL"],"messages":[{"role":"user","content":"EXAMPLE_BLOCK"}]}))' | \
curl -i http://127.0.0.1:3000/v1/chat/completions \
  -H "Authorization: Bearer $HUB_API_KEY" \
  -H 'Content-Type: application/json' \
  --data-binary @-
```

预期 HTTP 403，错误代码包含 `request_rejected`。插件页面出现请求阶段记录，action 为 reject；拒绝的请求不会进入渠道，也没有响应阶段记录。

响应替换和流式测试：

```bash
python3 -c 'import json,os; print(json.dumps({"model":os.environ["MODEL"],"stream":True,"messages":[{"role":"user","content":"EXAMPLE_PRIVATE。请只原样输出 EXAMPLE_SECRET"}]}))' | \
curl -N http://127.0.0.1:3000/v1/chat/completions \
  -H "Authorization: Bearer $HUB_API_KEY" \
  -H 'Content-Type: application/json' \
  --data-binary @-
```

请求中的 `EXAMPLE_PRIVATE` 被替换成 `[private]`。渠道实际输出 `EXAMPLE_SECRET` 时，客户端应看到 `[redacted]`；模型没有按要求复述时不能直接认定响应拦截失败。

回到插件页面按 request ID 关联 request 和 response 两阶段记录。只需要验证替换算法、并希望输入输出固定时，运行 example 已有 processor 测试或用可控 mock 渠道。

## 第六步 设置断点

本地调试包已禁用优化。安装 Delve 后，在插件运行信息中找到当前 PID，从终端执行以下命令；把 12345 替换为实际 PID：

```bash
cd "$HOME/work/example"
dlv attach 12345
```

`dlv attach` 用于接管已运行的进程，见 [Delve 官方说明](https://github.com/go-delve/delve/blob/master/Documentation/usage/dlv_attach.md)。在调试会话中设置断点并 continue。优先选择：

| 功能 | 断点位置 |
| --- | --- |
| 请求拒绝和替换 | `controller/relay.go` 的 `Service.Request`；`processor/rules.go` 的 `Rules.Request` |
| 响应和流式处理 | `controller/relay.go` 的 `Service.Response`；`processor/response.go` |
| 页面权限 | `controller/service.go` 的 `Service.RequireUser`；`hostclient/client.go` 的 `Client.Self` |
| 记录查询 | `controller/service.go` 的 `Service.Records` |

经过宿主的 relay 调用/流式交互有约 5 秒时限，长时间停断点可能导致 `plugin_timeout`。需要长时间单步时，先通过独立本地插件的内部协议请求复现，不增加共用宿主超时。

页面资源已包含 source map，可以在浏览器 DevTools 的 Sources 中定位 `web/src/plugin.jsx`。修改源码后需要重新构建、打包并安装对应版本，宿主运行的是安装目录中的副本。

## 只调试页面时使用 external

若目标只是开发插件页面和用户 API，可继续使用手动 `go run` 的服务。先在原运行终端 Ctrl+C，再把两个回调地址都切到本机：

```bash
cd "$HOME/work/example"
cp -n config.demo.yaml config.yaml
export EXAMPLE_HOST_SECRET="${EXAMPLE_HOST_SECRET:-$(openssl rand -hex 32)}"
export EXAMPLE_HOST_BASE_URL=http://127.0.0.1:3000
export EXAMPLE_HOST_DATA_BASE_URL=http://127.0.0.1:3000
export EXAMPLE_SERVER_ADDRESS=127.0.0.1:3012
export EXAMPLE_DATABASE_MODE=independent
export EXAMPLE_DATABASE_DRIVER=sqlite
export EXAMPLE_DATABASE_SQLITE_PATH=./data/example-local-external.db
go run . -config config.yaml
```

首次启动会生成通信密钥。接入宿主后，重启时复用同一 `EXAMPLE_HOST_SECRET`；换终端时加载先前保存的密钥，不生成与现有登记不一致的新值。上面的 SQLite 配置只读写插件本地数据。

external 安装时填写 URL `http://127.0.0.1:3012`、两个回调地址 `http://127.0.0.1:3000`，通信密钥与本机服务一致。external 与 process 不能以同一个 example ID 同时安装；切换模式需要按市场生命周期操作。

external 接入单机 master 后，不用本机 `/v1` 的模型请求验证拦截。该模式的页面与身份委托可以验证，拦截处理则使用直接 SDK 请求或额外的数据节点。

## 常见问题

| 现象 | 检查项 |
| --- | --- |
| callback 更新后仍请求 PoC | 安装时的 `host_url` 和 `data_host_url`、手动服务环境变量，以及实际连接的插件地址 |
| 浏览器切换地址后未登录 | 在 `127.0.0.1` 重新登录，并保持后续页面同一 origin |
| 安装后请求不被拒绝 | 运行模式是否 process；ZIP 的 `config.yaml` 是否来自 demo；是否仍连接旧安装 |
| 插件地址不再是 3012 | process 由部署器分配端口，按运行信息里的地址排查 |
| 命令报 3012 被占用 | 已有手动服务运行，在原终端停止后再启动 |
| API Key 有额度但请求失败 | 继续检查渠道、用户分组、模型权限和模型配置 |
| 有摘要而没有正文 | 宿主需要传递 `debug=true`；本地直接协议请求也可以显式设置该字段 |
| 新包与已有同版本内容冲突 | 按项目版本脚本设置新版本，重新构建包和目录，保持各 Manifest 一致 |

## 相关文件

- [插件清单](plugin.json)
- [演示规则配置](config.demo.yaml)
- [启动入口](main.go)
- [请求与响应拦截](controller/relay.go)
- [请求规则处理](processor/rules.go)
- [响应处理](processor/response.go)
- [前端页面](web/src/plugin.jsx)

宿主侧行为以配套 AI Hub 版本为准。同级 ai-hub 仓库中的 `common/extensions/plane.go` 定义节点与扩展点选择，`common/plugindeployer/process.go` 合并 process 配置模板，`web/src/views/Setting/component/PluginCatalogManagement.jsx` 实现目录登记界面。
