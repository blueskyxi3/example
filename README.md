# example：控制面页面与数据面拦截参考插件

独立 SDK v2 HTTP 服务，提供 `site.navigation`、`relay.request.intercept`、`relay.response.intercept`。默认规则为空，正文原样透传。项目只导入独立的 `ai-hub-plugin-api/relay` 通信包，不导入宿主业务包。SQLite/MySQL/PostgreSQL 中只迁移自己的 `example_relay_records` 表。

## 运行与构建

从本项目根目录执行：

```sh
cp config.demo.yaml config.yaml
# 生成通信密钥；master、slave 的注册 secret 必须与此相同。
export EXAMPLE_HOST_SECRET="$(openssl rand -hex 32)"
cd web
npm ci
npm run build
cd ..
go run . -config config.yaml
```

默认监听 `127.0.0.1:3012`，回调 master `http://127.0.0.1:3000`。容器部署请配置 `server.address: 0.0.0.0:3012`，并将 `host.base_url` 改为容器内可访问的 master origin。`config.example.yaml` 为无规则透传配置，`config.demo.yaml` 为可立即演示的拒绝／替换／脱敏配置。

```sh
make build
make test
```

构建新版本时运行 `make build VERSION=1.0.2`。构建前会同步更新 `plugin.json`、`catalog.example.json` 和 `registration.example.json` 中的版本及目录包路径；不传 `VERSION` 时沿用当前版本。构建后仍需将更新后的目录记录提交到宿主的插件目录。

`dist/example-<VERSION>-<GOOS>-<GOARCH>.zip` 是原生 process 安装包：根目录包含 `example`、`plugin.json`、`config.yaml`，前端嵌入可执行文件。必须在目标平台原生构建，SQLite 使用 CGO。本插件是独立 HTTP 程序，无需 Go `.so` ABI。目录样例的 `packages` 路径按同级 AI Hub 仓库根目录作为工作目录填写；若宿主从其他目录启动，请改为绝对路径。复制 `catalog.example.json` 中的目录记录到宿主插件目录，通过插件市场安装和授权；原生安装默认空规则，改配置后重启服务才生效。master 上的 process 插件会处理发往 master 的文本模型请求并记录两个拦截阶段。升级时原有密钥和数据库配置保留。

## master＋slave 共用同一个插件服务

单节点可直接使用 master 的 process 安装：控制面页面和发往 master 的数据面请求使用同一个插件进程。新增 slave 时，在 slave 显式登记 external 注册项，指向该插件进程或一份独立部署的同版本服务；两端必须共享同一插件数据库。process 进程默认只监听 `127.0.0.1`，跨主机的 slave 需通过私有网络代理提供可访问的插件 origin。

1. 在 master 安装 process 插件，或先部署独立 example 服务并在 master 以 external 安装。控制面回调 `host_url` 指向 master。
2. 在 slave 登记 external 注册项，插件地址和密钥与 master 使用的服务一致。`registration.example.json` 给出了启动注册文件结构；替换示例 origin 和密钥后合并到 slave 的注册文件，保留已有插件和授权。不要覆盖已有注册文件。
3. 将两端的 `host_url` 固定为 master 地址；master 的 `data_host_url` 指向 master，slave 注册项的 `data_host_url` 指向 slave。插件市场的 external 安装表单也可填写这两个回调地址。
4. 用户在 master 登录后打开顶部“插件示例”。模型请求发往处理请求的节点；页面通过 master 的插件代理读取同一服务产生的记录。

`host_url` 是控制面回调，`data_host_url` 是可选的数据面回调；省略后者时继承前者。插件的 `/internal/runtime` 同时返回和更新两个地址。数据面请求头中的地址仅用于该请求，不覆盖持久控制面地址。仅修改回调地址不会让 slave 自动调用插件，slave 必须有自己的 external 注册项。模型请求只传可信用户 ID/token ID/request ID 等元信息，不传 API Key、Cookie、渠道密钥。页面身份由宿主 HMAC 签名，权限每次用当前用户委托凭证查询 master 的 `/api/user/self`；普通用户强制限定本人，管理员查询全部。浏览器传入 `user_id` 或角色没有授权效力。

external 多实例使用同一通信密钥、同一 master 回调及共享 MySQL/PostgreSQL 数据库；SQLite 仅用于单插件实例演示。数据库配置示例：

```yaml
database:
  mode: independent
  driver: postgres
  dsn: "host=db port=5432 user=example password=CHANGE_ME dbname=example sslmode=require"
```

MySQL 使用 `driver: mysql` 和 Go MySQL driver DSN。`database.mode: shared` 表示复用宿主连接配置，仍然只读写插件自己的表；服务权限判断始终通过宿主 HTTP API。

## 演示请求

设置 `HUB_DATA_URL` 为 slave 地址，`HUB_API_KEY` 为真实用户 API Key，`MODEL` 为已配置文本模型：

```sh
export HUB_DATA_URL=http://127.0.0.1:3001
export MODEL=your-model
curl "$HUB_DATA_URL/v1/chat/completions" \
  -H "Authorization: Bearer $HUB_API_KEY" -H 'Content-Type: application/json' \
  -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"EXAMPLE_BLOCK\"}]}"
# 403：不会请求渠道或进入计费。

curl -N "$HUB_DATA_URL/v1/chat/completions" \
  -H "Authorization: Bearer $HUB_API_KEY" -H 'Content-Type: application/json' \
  -d "{\"model\":\"$MODEL\",\"stream\":true,\"messages\":[{\"role\":\"user\",\"content\":\"EXAMPLE_PRIVATE。请原样输出 EXAMPLE_SECRET\"}]}"
# 请求字面替换 EXAMPLE_PRIVATE → [private]；渠道输出 EXAMPLE_SECRET → [redacted]。
```

同样覆盖 `/v1/responses`、`/claude/v1/messages`、`/gemini/v1beta/models/<model>:generateContent`／`:streamGenerateContent`。响应钩子看到渠道原始协议，随后宿主进行客户端协议转换。工具参数、usage、模型及终止语义不可改写。拒绝规则在可识别文本块内匹配，替换采用左至右、同位置按配置顺序优先，替换结果不再次参与匹配。

## 流式与记录设计

每个响应与每个插件只建立一次双向 NDJSON 会话，处理完整 SSE 事件；使用独立 choice/内容块尾部窗口处理跨事件关键词。最多暂存 256 个事件／16 MiB，单 JSON 或事件上限 8 MiB；超限终止，不缓存整段生成。只保留可能成为匹配词的有限尾部，安全前缀立即返回；事件本身不能拆成部分输出，因此很小的文本事件可能等待下一个事件。Responses 的 text.done／completed 快照与增量使用同一替换规则。非文本字段保持原样。

反向代理必须关闭请求和响应缓冲；参见 `deploy/nginx.conf`，需要 HTTP/1.1 双向流式。服务在原始 HTTP writer 上启用 full duplex；集成测试使用真实 HTTP/1.1 验证首包与会话收尾。对不支持的渠道编码（NDJSON、二进制、音视频、WebSocket 等）明确返回本地错误。无终止事件的意外 EOF、中途失败、协议违规及超时不能当作成功；宿主不会因此重试或禁用渠道。已发出的文本不能撤回。

每个 request ID＋扩展点＋渠道尝试创建一条汇总记录，开始时插入、结束时更新，中间最多每秒更新一次。页面按请求 ID 关联两阶段，通过后端查询判断响应阶段是否已执行，支持筛选和分页；仅可见页面中有执行中记录时每两秒刷新。客户端取消关闭所有会话并释放租约，不逐 token 写数据库，不提供完整流回放。

宿主所有日志级别保存审计摘要（`logs.type=6`），不依赖消费日志开关，不产生消费金额。仅宿主 `log_level: debug` 向插件传递 `debug=true` 保存正文，每份最多 64 KiB；宿主单请求全部正文再限制到 512 KiB。流式保存最后有效内容事件，终止标记单独记录。页面非 debug 明确显示“当前日志级别未保存正文”。处理错误使用安全代码，panic 堆栈只写服务端日志；落库失败报告错误，不声称已保存。

## 验证

```sh
# 独立 SDK 和插件测试
(cd ../ai-hub/common/pluginapi && go test ./relay)
go test ./... -count=1
(cd web && npm test && npm run build)
# 原生包构建后，在同级 AI Hub 仓库根目录执行真实安装／重启＋external 数据面测试
cd ../ai-hub
AI_HUB_EXAMPLE_TEST_PACKAGE="$PWD/../example/dist/example-1.0.2-$(go env GOOS)-$(go env GOARCH).zip" \
  go test ./controller -run '^TestMarketplaceNativeExample' -count=1
```

参考 SDK v2 文档第 14 节，了解 JSON/NDJSON 帧、错误语义、超时和受保护字段。测试使用 SQLite；MySQL/PostgreSQL 共用 GORM 查询和模型，部署时另行验证实际服务连接。Nginx 配置是部署模板，须在实际代理环境验证无缓冲与取消传播。

当前实现的本地验证环境为 Go 1.26.4、darwin-arm64。宿主 extensions/requester/middleware/relay/relay_util/router/model 包回归、插件独立测试、流式竞态检查、前端构建，以及真实原生包安装／重启／slave external 调用已通过。`controller` 插件市场与管理用例通过；扩大到全包运行时，现有两个用户组测试因测试库缺少 `audit_events` 失败，独立运行可复现，本次未修改这些用例。依赖专用 Redis 的集成用例未配置环境而跳过；不将计费单元回归等同于真实 Redis 验收。MySQL/PostgreSQL 与 Nginx 实际部署仍按目标环境验收。
