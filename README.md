# Quota Decision Plane

纯Go分布式配额与速率决策服务，为API网关和内部服务提供低延迟`Check`、`Reserve`、`Commit`、`Cancel`与`BatchCheck`语义。默认以进程内适配器运行，PostgreSQL迁移定义了生产持久化边界；热路径不会等待审计消费者。

## 能力边界

- 固定窗口、滑动窗口、令牌桶、漏桶、并发信号量，以及父子链层级配额。
- 服务、方法、区域和客户四维通配匹配，优先选择更具体的定义。
- 预留带TTL，提交后不可取消；取消或超时会退还父子链上的全部占用。
- 所有决策写请求要求幂等键。相同键和相同正文返回缓存结果并设置`X-Idempotent-Replay: true`；正文变化返回HTTP 409。
- 配置草稿执行ID、引用、父级循环和匹配范围冲突检测，发布原子切换活动版本，回滚产生新的不可变版本。
- 一致性哈希环提供128虚拟节点、健康权重和副本选择；节点租约每次换主递增fencing epoch。
- 本地令牌租借与中心计数校正组件支持有界超发，强一致模式不允许`max_overage`。
- 审计通过有界异步队列写入，慢消费者不会阻塞决策；丢弃量暴露为指标。

本项目不实现特性开关、告警规则或通用策略语言。内存适配器用于可重复的本地运行；迁移中的PostgreSQL表用于配额版本、预留、租约、幂等结果和审计持久化。没有配置外部数据库时，不宣称跨进程重启保留状态。

## 一致性模型

`strong`定义由当前分片持有者的有效节点租约和fencing epoch保护。调用方必须拒绝低于已观察epoch的结果。`bounded`定义可通过本地令牌租借减少中心往返，最多允许配置的`max_overage`，节点失联时切换为保守判定，不再使用超发额度。配置发布是单进程原子活动指针切换；多节点部署需将同一版本事件分发给所有节点并在中心存储上CAS活动版本。

固定/滑动窗口使用注入时钟计算边界，令牌/漏桶只按非负经过时间补充或泄漏。生产时钟适配器应将墙钟校正映射到单调时间，禁止因NTP回拨增加额度。

## 目录

```text
cmd/server                 HTTP服务入口
cmd/load-simulator         并发热键压测器
internal/quota             定义、维度匹配和决策编排
internal/algorithm         六类算法及状态
internal/reservation       TTL预留状态机与回收器
internal/configuration     草稿、静态校验、发布和回滚
internal/shard             一致性哈希分片
internal/lease             节点租约和fencing epoch
internal/distribution      本地令牌租借与中心校正
internal/idempotency       请求摘要和结果重放
internal/audit             非阻塞审计总线
internal/platform          配置、HTTP中间件、指标和运行时装配
api                        OpenAPI和protobuf契约
configs/migrations/deploy  配置、数据库模式和容器交付
scripts                    冒烟脚本
```

各业务域按`domain`、`application`、`adapter`和`infrastructure`边界组织。当前HTTP适配器集中在platform域，存储实现位于各域的infrastructure包；空的可替换边界目录保留给gRPC和PostgreSQL实现。

## 启动

需要Go 1.22或更高版本。

```bash
go test ./...
go vet ./...
go build ./...
go run ./cmd/server -config configs/local.yaml
```

服务监听`127.0.0.1:18333`对应的配置值`:18333`。健康、就绪和指标无需认证，其他端点要求`X-API-Key: dev-secret`。配置项均可由`QUOTA_ADDRESS`、`QUOTA_NODE_ID`、`QUOTA_API_KEY`、`QUOTA_REQUEST_TIMEOUT`、`QUOTA_MAX_BODY_BYTES`和`QUOTA_RATE_LIMIT`覆盖。

```bash
./scripts/smoke.sh
go run ./cmd/load-simulator -requests 100 -workers 8
```

## 决策示例

```bash
curl -sS -H 'X-API-Key: dev-secret' -H 'Content-Type: application/json' \
  -d '{"tenant_id":"demo","resource":"api_calls","dimensions":{"service":"payments","method":"POST","region":"ap-northeast-1","customer":"c-1"},"cost":1,"idempotency_key":"check-001"}' \
  http://127.0.0.1:18333/v1/check
```

内置定义中，`payments-post`令牌桶是`global-api`固定窗口的子配额，因此请求需要同时获得两层额度。响应提供命中的配额、限制、已用、剩余、重试时间、配置版本、模式和fencing epoch。

`Check`会消费额度。只评估不消费可通过`BatchCheck`设置`consume:false`执行。预留适合稍后确认资源占用的调用：

```bash
curl -sS -H 'X-API-Key: dev-secret' -H 'Content-Type: application/json' \
  -d '{"tenant_id":"demo","resource":"jobs","dimensions":{"service":"worker","region":"ap-northeast-1"},"cost":1,"ttl_ms":30000,"idempotency_key":"reserve-001"}' \
  http://127.0.0.1:18333/v1/reserve

curl -sS -H 'X-API-Key: dev-secret' -H 'Content-Type: application/json' \
  -d '{"idempotency_key":"commit-001"}' \
  http://127.0.0.1:18333/v1/reservations/RESERVATION_ID/commit
```

取消将`pending`转为`cancelled`并退还额度；提交将其转为`committed`且不可再取消。同一动作重复提交由幂等层返回第一次结果。

## 配置发布与回滚

配额定义中的`window`按Go duration的纳秒整数编码，例如一分钟为`60000000000`。创建草稿时可省略definitions以复制base version，也可提交完整定义集合。

```bash
curl -sS -H 'X-API-Key: dev-secret' -H 'Content-Type: application/json' \
  -d '{"id":"draft-2","base_version":1,"note":"raise search capacity","definitions":[{"id":"search-v2","tenant_id":"demo","resource":"search","version":1,"algorithm":"sliding_window","limit":40,"window":60000000000,"dimensions":{"service":"catalog","method":"GET","region":"*","customer":"*"},"mode":"bounded","max_overage":2,"enabled":true}]}' \
  http://127.0.0.1:18333/v1/config/drafts

curl -sS -X POST -H 'X-API-Key: dev-secret' http://127.0.0.1:18333/v1/config/drafts/draft-2/validate
curl -sS -X POST -H 'X-API-Key: dev-secret' http://127.0.0.1:18333/v1/config/drafts/draft-2/publish
curl -sS -H 'X-API-Key: dev-secret' -H 'Content-Type: application/json' -d '{"version":1}' http://127.0.0.1:18333/v1/config/rollback
```

回滚不会修改历史版本，而是从目标复制定义并发布一个递增的新版本。`POST /v1/config/shadow`选择历史版本后，每个正常决策还会返回不消费额度的`shadow`结果。

## 运维与故障演练

- `GET /v1/shards/{key}`查看热键的主节点、副本和环epoch。
- `GET /v1/leases`查看已处理分片的持有者、fencing epoch和到期时间。
- `GET /v1/audit/events?limit=100`查看决策与预留状态审计。
- 停止主节点后，将相同节点集合中的主节点标记不健康并重建环；新主租约epoch必须增加。
- 重放相同幂等请求验证结果不重复消费；修改cost后重放应收到409。
- `quota_audit_dropped_total`增长表示审计消费者持续落后，需要扩容或修复sink。

容器方式：

```bash
docker compose -f deploy/docker-compose.yaml up --build
docker compose -f deploy/docker-compose.yaml down -v
```

Compose包含PostgreSQL以供实现持久化适配器时执行`migrations/001_initial.up.sql`。当前默认运行时仍明确使用内存仓储，避免误报尚未接线的数据库一致性能力。

## 验收清单

1. `/healthz`、`/readyz`返回200，`/metrics`包含请求和决策计数。
2. `Check`达到限制后拒绝并提供`retry_after_ms`；父配额拒绝时已消费的子额度会补偿。
3. `Reserve`返回ID，`Commit`确认，`Cancel`退还，TTL回收器自动释放过期预留。
4. 同正文重放返回`X-Idempotent-Replay`，不同正文使用同一键返回409。
5. `BatchCheck`保持输入顺序并限制100项。
6. 冲突草稿不能发布；合法草稿发布后`config_version`更新；回滚创建新版本。
7. 热键分片稳定，主节点变化产生更高fencing epoch；审计积压不会阻断决策。

