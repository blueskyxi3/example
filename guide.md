# example 插件本地 external 调试指南

本地开发使用 external 模式：TiTu Edge 宿主运行在 3000，example 由开发者通过 `go run`、VS Code 或 Delve 启动在 3012。宿主连接这个服务，修改 Go 代码后重启调试进程即可继续验证。

默认目录为 `~/work/example` 和同级 `~/work/ai-hub`。example 的 `go.mod` 通过相对路径引用宿主的独立 SDK。宿主需支持 SDK v2 和插件市场；本机需有 Go、Node、npm 和可编译 SQLite CGO 的 C 工具链。

## 地址与调试范围

| 项目 | 本地值 |
| --- | --- |
| 宿主 origin | `http://127.0.0.1:3000` |
| 模型客户端的 API Base URL | `http://127.0.0.1:3000/v1` |
| example 服务 URL | `http://127.0.0.1:3012` |
| 控制面回调 host_url | `http://127.0.0.1:3000` |
| 数据面回调 data_host_url | `http://127.0.0.1:3000` |
| 插件管理 | `http://127.0.0.1:3000/panel/plugin-management` |
| 插件市场 | `http://127.0.0.1:3000/plugin-marketplace` |
| example 页面 | `http://127.0.0.1:3000/plugins/example` |

回调地址使用宿主 origin，**不带 `/v1` 或 `/api`**。浏览器登录、登记和查看页面时统一使用 `127.0.0.1`；从 `localhost` 切换过来时可能需要重新登录。

本流程验证宿主加载页面、用户身份委托，以及直接 SDK 请求中的拒绝、替换和响应脱敏。当前宿主对单机 master 的 external 登记只启用控制面扩展点；模型拦截的真实网关链路见文末“数据面联调”。

## 第一步 准备前端和本地配置

在终端执行：

```bash
cd "$HOME/work/example/web"
npm ci
npm run build -- --sourcemap

cd "$HOME/work/example"
mkdir -p data
if [ ! -f data/config.local-debug.yaml ]; then
  cp config.demo.yaml data/config.local-debug.yaml
fi

if [ ! -f data/.env.local-debug ]; then
  (
    umask 077
    printf 'EXAMPLE_HOST_SECRET=%s\n' "$(openssl rand -hex 32)" > data/.env.local-debug
    cat >> data/.env.local-debug <<'ENV'
EXAMPLE_SERVER_ADDRESS=127.0.0.1:3012
EXAMPLE_HOST_BASE_URL=http://127.0.0.1:3000
EXAMPLE_HOST_DATA_BASE_URL=http://127.0.0.1:3000
EXAMPLE_DATABASE_MODE=independent
EXAMPLE_DATABASE_DRIVER=sqlite
EXAMPLE_DATABASE_SQLITE_PATH=./data/example-local-external.db
ENV
  )
fi
```

`data/` 已被 Git 忽略，本地密钥、可写配置和 SQLite 都保存在该目录。环境文件只在首次执行时生成；后续重启复用同一通信密钥。旧配置文件已存在时，检查其中的演示规则是否仍符合本次调试需求。

前端嵌入 Go 程序，因此首次启动前需要先构建 `web/build`。本地配置使用 [config.demo.yaml](config.demo.yaml)，包含以下规则：

| 阶段 | 匹配文本 | 处理结果 |
| --- | --- | --- |
| 请求拒绝 | `EXAMPLE_BLOCK` | `request_rejected` |
| 请求替换 | `EXAMPLE_PRIVATE` | `[private]` |
| 响应替换 | `EXAMPLE_SECRET` | `[redacted]` |

## 第二步 启动本机 example

保持 TiTu Edge 桌面宿主运行。3012 已有旧的手动服务时，先在它的运行终端按 Ctrl+C，再启动当前配置：

```bash
cd "$HOME/work/example"
set -a
source data/.env.local-debug
set +a
go run . -config data/config.local-debug.yaml
```

看到 `example service listening` 后保留该终端。启动使用独立的本地 SQLite，初始化插件已有的 `example_relay_records` 表。

另开终端，加载相同环境并验证服务：

```bash
cd "$HOME/work/example"
set -a
source data/.env.local-debug
set +a

curl --fail http://127.0.0.1:3012/healthz
curl --fail http://127.0.0.1:3012/internal/manifest \
  -H "X-AIHub-Plugin-Secret: $EXAMPLE_HOST_SECRET"
curl --fail http://127.0.0.1:3012/internal/runtime \
  -H "X-AIHub-Plugin-Secret: $EXAMPLE_HOST_SECRET"
```

预期依次得到 `{"status":"ok"}`、本地插件 Manifest 和两个指向 3000 的回调地址。`/healthz` 检查数据库连通性；页面查询还需要后续的宿主身份委托链路。

## 第三步 生成并登记 external 目录

在项目根目录生成目录记录：

```bash
cd "$HOME/work/example"
python3 - <<'PY'
import json
from pathlib import Path

root = Path.cwd()
catalog = {
    'manifest': json.loads((root / 'plugin.json').read_text()),
    'description': '本地 external 开发与断点调试',
    'modes': ['external'],
    'external_url': 'http://127.0.0.1:3012',
    'host_url': 'http://127.0.0.1:3000',
    'data_host_url': 'http://127.0.0.1:3000',
    'trial_days': 7,
    'purchase_days': 0,
    'rollback_compatible': False,
}
target = root / 'data/catalog.local-debug.json'
target.write_text(json.dumps(catalog, ensure_ascii=False, indent=2) + '\n')
print(target)
PY
```

目录声明只开放 external，Manifest 与本机源码一致，目录 JSON 不保存通信密钥。

1. 用有插件管理权限的管理员登录本机宿主。
2. 进入“插件管理 → 插件目录”，点击“登记插件”；已有 example 目录项时编辑该项。
3. 粘贴 `~/work/example/data/catalog.local-debug.json` 的完整内容并保存。
4. 进入插件市场，找到“插件示例”，按现有授权选择安装、试用或购买并安装。

安装时填写：

| 字段 | 填写值 |
| --- | --- |
| 安装模式 | 外部服务 external |
| 服务 URL | `http://127.0.0.1:3012` |
| 控制面回调 | `http://127.0.0.1:3000` |
| 数据面回调 | `http://127.0.0.1:3000` |
| 通信密钥 Secret | `data/.env.local-debug` 中的 EXAMPLE_HOST_SECRET |

external 服务的数据库、规则和监听地址由本机配置控制。宿主登记中的通信密钥必须与本机服务相同。

已有 external 安装时，核对其运行信息和本机密钥即可继续调试。已有 process 安装时，先记录原配置，再按市场生命周期停用、卸载后重新安装 external；修改目录不会改变已安装插件的模式。原安装的数据与新的本地 SQLite 是各自独立的。

## 第四步 验证页面和用户 API

刷新整个宿主页面，打开 `http://127.0.0.1:3000/plugins/example`。在浏览器 DevTools 的 Network 中检查：

1. `GET /api/plugins` 返回的插件列表包含 example。
2. `GET /api/plugin-bridge/example/assets/plugin.js?version=...` 返回本机前端模块。
3. `GET /api/plugin-bridge/example/api/v1/records?page=0&size=20` 返回 `success=true`。

页面请求由宿主代理到 3012，再由插件携带委托身份回调宿主 `/api/user/self`。管理员可看所有执行记录，普通用户只看自己的记录。

插件页面使用宿主提供的 React、MUI 和 API 客户端，没有独立首页或 `npm run dev` 脚本。访问 `http://127.0.0.1:3012/` 返回 404 属于正常情况；业务 API 通过宿主 plugin-bridge 访问。

## 第五步 使用 VS Code 或 Delve 设置断点

使用 VS Code 打开 `~/work/example`，安装 Go 扩展，通过 `Go: Install/Update Tools` 安装或更新 `dlv`。在 `.vscode/launch.json` 中合并：

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "example local external",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}",
      "cwd": "${workspaceFolder}",
      "args": ["-config", "data/config.local-debug.yaml"],
      "envFile": "${workspaceFolder}/data/.env.local-debug"
    }
  ]
}
```

停止前面的 `go run`，再按 F5，避免两个服务争用 3012。宿主仍连接同一 URL 和密钥，后续 Go 代码修改只需重新启动调试会话。

也可在加载过环境文件的终端直接使用：

```bash
cd "$HOME/work/example"
set -a
source data/.env.local-debug
set +a
dlv debug . -- -config data/config.local-debug.yaml
```

进入 Delve 后设置断点并执行 `continue`，服务才会开始处理请求。配置说明见 [VS Code Go 官方文档](https://github.com/golang/vscode-go/blob/master/docs/debugging.md)和 [Delve 官方文档](https://github.com/go-delve/delve/blob/master/Documentation/usage/dlv_debug.md)。

| 调试目标 | 断点位置 |
| --- | --- |
| 启动配置与规则 | main.go 的 run |
| 用户身份与权限 | controller/service.go 的 RequireUser；hostclient/client.go 的 Self |
| 页面记录查询 | controller/service.go 的 Records |
| 请求拒绝和替换 | controller/relay.go 的 Request；processor/rules.go 的 Rules.Request |
| 响应处理 | controller/relay.go 的 Response；processor/response.go |

## 第六步 直接发送 SDK 请求验证拦截逻辑

以下请求直接调用本机插件，记录写入本地 SQLite。元数据中的用户 ID 1 是模拟值；需要以普通用户查看这些记录时，替换为该用户实际 ID。管理员可查看全部记录。

先在测试终端加载第二步的环境文件，再执行拒绝和替换测试：

```bash
for EXAMPLE_TEST_TEXT in EXAMPLE_BLOCK EXAMPLE_PRIVATE; do
  export EXAMPLE_TEST_TEXT
  python3 - <<'PY' | curl -i http://127.0.0.1:3012/internal/relay/request \
    -H "X-AIHub-Plugin-Secret: $EXAMPLE_HOST_SECRET" \
    -H 'Content-Type: application/json' --data-binary @-
import json
import os
import uuid

print(json.dumps({
    'metadata': {
        'request_id': 'example-local-' + uuid.uuid4().hex,
        'user_id': 1, 'token_id': 1, 'protocol': 'openai',
        'model': 'debug-model', 'stream': False, 'debug': True, 'attempt': 1,
    },
    'body': {
        'model': 'debug-model',
        'messages': [{'role': 'user', 'content': os.environ['EXAMPLE_TEST_TEXT']}],
    },
}))
PY
done
```

两次内部接口调用均预期 HTTP 200：第一条 JSON 中 `summary.action=reject`、`error.code=request_rejected`；第二条 `summary.action=replace`，返回正文包含 `[private]`。内部协议通过 JSON 表达拒绝；真正经过宿主 relay 时，宿主才将拒绝转换为客户端 HTTP 403。

响应脱敏可用固定 JSON 响应模拟，命中 Response 与响应 processor 的断点：

```bash
python3 - <<'PY' | curl -sS -N http://127.0.0.1:3012/internal/relay/response \
  -H "X-AIHub-Plugin-Secret: $EXAMPLE_HOST_SECRET" \
  -H 'Content-Type: application/x-ndjson' --data-binary @- | \
python3 -c '
import base64, json, sys
for line in sys.stdin:
    frame = json.loads(line)
    print(frame["type"], frame.get("summary", {}))
    for unit in frame.get("outputs", []):
        print(base64.b64decode(unit["data"]).decode())
'
import base64
import json
import uuid

metadata = {
    'request_id': 'example-response-' + uuid.uuid4().hex,
    'user_id': 1, 'token_id': 1, 'protocol': 'openai',
    'model': 'debug-model', 'stream': False, 'debug': True, 'attempt': 1,
    'status_code': 200, 'content_type': 'application/json',
}
body = json.dumps({'choices': [{'index': 0, 'message': {
    'role': 'assistant', 'content': 'EXAMPLE_SECRET',
}, 'finish_reason': 'stop'}]}).encode()
frames = [
    {'type': 'begin', 'metadata': metadata},
    {'type': 'data', 'unit': {'sequence': 1, 'data': base64.b64encode(body).decode()}},
    {'type': 'end'},
]
for frame in frames:
    print(json.dumps(frame))
PY
```

预期收到 ready、ack、end 帧，解码后的响应正文包含 `[redacted]`，summary 的 action 为 replace。回到宿主 example 页面可查询到相应执行记录。更多流式边界场景可运行现有测试：

```bash
go test ./processor ./router -count=1
```

通过宿主调用时，relay 交互和用户权限回调有超时约束；长时间停断点可能导致请求失败。直接 SDK 请求便于在本机进行较长时间的单步排查。

## 日常修改和重启

- Go 代码或规则修改后，重启 `go run` 或 VS Code 调试会话。
- 前端修改后执行 `cd web && npm run build -- --sourcemap`，再重启 Go 调试进程并整页刷新宿主。
- 前端静态资源嵌入在 Go 程序中；宿主也会缓存动态导入模块，单纯切换路由不会刷新模块。可在 DevTools 启用 Disable cache。
- URL、Manifest 版本和密钥保持一致时，可以沿用宿主上的 external 登记。Manifest 版本变化时，更新目录并按市场升级流程同步登记。
- 所有终端与 IDE 使用同一个 `data/.env.local-debug`；更改通信密钥需要同步更新宿主登记。

## 数据面联调

当前 AI Hub 的 external 扩展点按宿主角色执行：master 加载页面等控制面能力，slave 执行请求和响应拦截。单机桌面 master 上登记 external 后，向 `http://127.0.0.1:3000/v1` 发请求，不能直接作为拦截生效的验收条件。

需要验证真实模型网关链路时，保持 example 以 external 运行，在同一套用户和令牌体系的数据节点上也登记同一插件 URL 和通信密钥。控制面回调始终指向 master 的 3000；数据面回调填写实际 slave 地址，例如本地 slave 的 `http://127.0.0.1:3001`。模型请求发送到该数据节点的 `/v1/chat/completions`。

数据节点必须已经启动并加载登记；只把 data_host_url 改为 3001 不会启动数据节点或自动登记。master 与数据节点连接同一个 example 服务和数据库，页面才能看到相应拦截记录。

## 常见问题

| 现象 | 检查项 |
| --- | --- |
| 启动报 web/build 缺失 | 先构建前端 |
| 3012 被占用 | 停止旧的手动进程或调试会话 |
| internal 接口返回 401 | 测试终端、插件和宿主使用的通信密钥是否一致 |
| 页面入口不存在 | example 是否已授权、启用，以及 /api/plugins 是否返回该插件 |
| 页面查询返回 503 | 插件回调是否仍指向旧 PoC；宿主用户委托与 /api/user/self 是否正常 |
| 直接访问 records 返回 401 | 用户 API 需要宿主委托身份，通过本机宿主页面访问 |
| 页面正常但本机 /v1 没有拦截记录 | 检查数据面联调的角色与登记条件 |
| 演示规则没有命中 | 检查 data/config.local-debug.yaml，修改后重启插件 |
| 改完前端仍是旧页面 | 重新构建前端、重启 Go 并整页刷新宿主 |

## 相关文件

- [插件清单](plugin.json)
- [演示规则配置](config.demo.yaml)
- [启动入口](main.go)
- [路由与内部协议接口](router/router.go)
- [请求与响应拦截](controller/relay.go)
- [请求规则处理](processor/rules.go)
- [响应处理](processor/response.go)
- [前端页面](web/src/plugin.jsx)

宿主节点与扩展点选择规则位于同级 ai-hub 仓库的 `common/extensions/plane.go`。本地 SDK 协议定义位于 `common/pluginapi/relay/protocol.go`，真实网关联调以配套宿主版本为准。
