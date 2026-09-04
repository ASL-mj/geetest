# CaptchaFlow 验证码服务平台 V1 设计规格

**状态：** 已确认产品方案，待进入实施计划  
**日期：** 2026-09-04  
**目标读者：** 产品、后端、前端、测试、运维

## 1. 目标与范围

将现有无状态解析服务封装为一个多用户开发者平台。平台对外以 CaptchaFlow 品牌提供用户 API Key 和统一的验证码解析接口，并负责 CDK 激活/再次进入、配额、限流、幂等、调用审计和管理员运营。

V1 交付以下能力：

- CDK 激活、CDK 再次进入、退出会话；不设置用户密码；
- 用户 API Key 的创建、重命名、启用、禁用和删除；
- 单一 `slide` 类型的验证码解析接口；
- 用户/CDK 维度共享的调用额度、每分钟速率和并发限制；
- 不可变配额流水、调用日志和管理员操作审计；
- 用户控制台、管理员后台、接口文档和在线调试；
- 解析服务健康检查和平台级可观测性。

V1 不包含在线支付、自动续费、多验证码类型、团队与子账号、Webhook 回调、异步批量任务和多节点调度。

## 2. 系统边界

```text
用户程序 / 用户端浏览器 / 管理员浏览器
                 |
                 v
      CaptchaFlow 服务平台业务后端
       |        |          |
 PostgreSQL   Redis   平台前端/管理端
       |
       v
已部署 GeeTest 解析服务
```

### 2.1 已部署 GeeTest 解析服务

- 对内健康检查：`GET /healthz`。
- 对内解析接口：`POST /v1/geetest/solve`。
- 对内认证：`X-Service-Key: SERVICE_API_KEY`。
- 职责仅限验证码解析，不保存用户、CDK、平台 API Key、额度、订单或调用记录。

### 2.2 本次建设的平台业务后端

- 唯一持有 `GEETEST_SOLVER_URL` 与 `GEETEST_SERVICE_API_KEY` 服务端配置。
- 对外暴露平台 API，不转发底层认证头，也不返回底层原始错误。
- 完成用户、CDK、API Key、配额、限流、并发、日志、审计和健康状态聚合。

### 2.3 用户端与管理员后台

- 浏览器只调用平台 API，禁止直接调用底层解析服务。
- 用户端和管理员端共享视觉语言、状态字典、表格组件和权限模型。
- 平台公网入口必须使用 HTTPS；底层服务地址不写入前端构建产物。

## 3. 角色与授权

| 角色 | 权限 |
|---|---|
| 访客 | 输入 CDK 激活或再次进入。 |
| 普通用户 | 查看自己的 CDK 身份、Key、用量、日志和文档；调用平台解析接口。 |
| 管理员 | 管理 CDK 批次、CDK、用户、Key、额度、日志、系统设置和服务健康状态。 |
| 管理员只读角色 | 查看仪表盘、日志、账本和健康状态；没有任何写权限。 |

普通用户只能访问自身数据。管理员接口使用独立会话和独立权限校验，管理员的每次写操作必须产生审计记录。

## 4. 领域规则

1. V1 中一个 CDK 只绑定一个用户，一个用户只绑定一个服务 CDK。
2. CDK 的 `activation_deadline` 是激活截止时间；`expires_at` 是已激活服务的到期时间。
3. Key 归属用户，额度归属用户/CDK，多个 Key 共享同一额度池。
4. CDK 禁用、到期或额度耗尽时，所有关联 Key 的有效调用状态立即失效。
5. Key 本身不需要批量改写状态；每次认证均计算其有效状态：`key active && user active && cdk active && not expired && quota available`。
6. Key 明文只在创建响应中返回一次；数据库仅保存 HMAC 哈希、前缀和末尾四位。
7. 解析成功消耗一单位额度；系统错误、下游 5xx 和超时自动退回额度。
8. 请求参数错误、鉴权失败、限流、并发超限和幂等重试不消耗额度。
9. `Idempotency-Key` 在同一用户和同一操作内唯一；重复请求不会再次调用底层服务。
10. 历史日志保存 Key 名称和前缀快照；删除 Key 后历史调用保持可检索。

## 5. 用户端信息架构

| 页面 | 展示字段 | 操作 |
|---|---|---|
| CDK 激活 / 进入 | CDK、服务状态、错误提示 | 激活、再次进入、退出。 |
| 控制台首页 | 服务状态、CDK 状态、到期时间、总/已用/剩余额度、今日调用、成功率、最近调用、API 地址 | 复制地址和示例、进入 Key 管理。 |
| API Key 管理 | 名称、前缀、末尾四位、状态、创建时间、最后调用、调用次数 | 创建、重命名、启用、禁用、删除。 |
| 在线接口调试 | 已选 Key、`captcha_id`、`risk_type`、脱敏响应、请求 ID、耗时、配额变化 | 发起调试、复制请求 ID。 |
| 用量统计 | 时间范围、调用趋势、成功率、按 Key 聚合、额度流水摘要 | 筛选时间和 Key。 |
| 调用日志 | 请求 ID、Key、Captcha ID、状态、耗时、错误码、扣费状态、时间 | 筛选、分页、复制、查看详情。 |
| 接口文档 | 平台地址、认证、请求/响应示例、错误码 | 复制 curl、Python、JavaScript 示例。 |
| 账号与服务信息 | CDK 身份、CDK 批次、服务状态、到期时间、会话状态 | 重新输入 CDK、退出会话。 |

在线调试通过受控的平台接口执行，与公开解析接口共用认证后的 Key、限流、并发、额度与审计链路。浏览器不显示底层服务密钥。

## 6. 管理端信息架构

| 页面 | 核心数据 | 写操作 |
|---|---|---|
| 数据概览 | 用户数、活跃用户、调用量、成功率、异常量、额度分布 | 无。 |
| CDK 批次 | 名称、默认额度、激活期限、服务期限、创建人 | 新建、批量生成、导出。 |
| CDK 管理 | 编码前缀、状态、绑定用户、额度、激活/到期、最后使用 | 禁用、作废、查看绑定。 |
| 用户管理 | 用户、CDK、状态、额度、调用量、最后登录 | 调整额度、封禁、解封。 |
| API Key 管理 | Key 前缀、用户、状态、调用量、最后使用 | 启用、禁用、定位用户。 |
| 调用日志 | 用户、Key、Captcha ID、状态、错误码、耗时、请求 ID | 筛选、导出脱敏数据。 |
| 配额流水 | 变动前、变动值、变动后、类型、原因、请求 ID | 查询。 |
| 系统设置 | 默认额度、Key 上限、默认速率、默认并发、公告、平台地址 | 修改设置。 |
| 管理员审计 | 管理员、操作、目标、原因、前后值、IP、时间 | 查询。 |
| 解析服务状态 | 健康、最近检查、延迟、失败次数 | 手动检查。 |

管理员的禁用、作废、额度调整和系统设置修改必须要求确认，并要求填写可审计原因。

## 7. 核心业务流程

### 7.1 CDK 激活

```text
访客提交 CDK
  -> 校验 CDK 格式、状态、激活截止时间、额度和绑定状态
  -> 事务锁定 CDK
  -> 首次使用时创建内部用户
  -> 绑定 CDK，设置 activated_at 与 expires_at
  -> 创建默认 API Key
  -> 写入激活/进入审计记录
  -> 创建用户浏览器会话
  -> 仅本次响应返回默认 API Key 明文
```

同一 CDK 的并发首次激活依赖数据库行锁和 `bound_user_id` 唯一约束；已绑定的 CDK 再次提交时只签发新的浏览器会话，不创建第二个用户或 Key。

### 7.2 公开解析调用

```text
用户程序带 Bearer API Key 和 Idempotency-Key 请求平台
  -> 验证 API Key、用户、CDK、服务有效期
  -> 校验请求体
  -> 查询或创建幂等调用记录
  -> 执行速率和并发检查
  -> 原子预扣一单位额度，并写 RESERVE 流水
  -> 平台后端调用底层 GeeTest 解析服务
  -> 成功：确认额度、写调用结果、返回标准化响应
  -> 下游失败：退回额度、写失败日志、返回标准错误
```

下游 HTTP 调用不处于数据库事务内。事务只覆盖状态变更和账本写入，避免将数据库锁持有到网络请求结束。

### 7.3 幂等重试

1. 使用 `(user_id, operation, idempotency_key_hash)` 唯一约束写入调用记录。
2. 已完成的同键请求返回首次保存的标准化状态、响应体和 `request_id`。
3. 执行中的同键请求等待至平台同步超时；仍未完成时返回 `409 IDEMPOTENCY_IN_PROGRESS`，不重新预扣和下游调用。
4. 客户端可通过 `GET /v1/calls/{request_id}` 查询最终状态。

## 8. 状态模型

### 8.1 CDK

```text
UNACTIVATED -> ACTIVE -> EXPIRED
                    |        |
                    v        v
                 DISABLED  DISABLED
```

`EXHAUSTED` 是 `quota_remaining = 0` 导出的服务状态。管理员补充额度后，状态自动恢复为 `ACTIVE`，前提是 CDK 未过期且未禁用。

### 8.2 API Key

```text
ACTIVE <-> DISABLED
ACTIVE / DISABLED -> DELETED
```

`DELETED` 为软删除终态。Key 的有效调用状态还受用户、CDK、有效期和额度影响。

### 8.3 调用

```text
RECEIVED -> REJECTED
RECEIVED -> RESERVED -> DISPATCHED -> SUCCEEDED
                                  -> FAILED_REFUNDED
```

`REJECTED` 不产生配额预扣。`FAILED_REFUNDED` 的退款动作必须可重试且最多成功一次。

## 9. 平台 API 契约

### 9.1 通用规则

- 平台公开入口：`https://api.example.com`，通过系统设置配置。
- 所有时间均为 ISO 8601 UTC 时间戳。
- 日志列表使用 `cursor` 和 `limit`，`limit` 最大 100。
- 成功与失败响应都包含平台 `request_id`。
- 公开解析接口的 Key 格式为 `cf_live_<random>`。

### 9.2 用户会话接口

| 方法 | 路径 | 认证 |
|---|---|---|
| POST | `/v1/auth/activate` | 无。 |
| POST | `/v1/auth/logout` | 用户会话。 |
| GET | `/v1/account` | 用户会话。 |
| GET | `/v1/usage` | 用户会话。 |
| GET | `/v1/calls` | 用户会话。 |
| GET | `/v1/calls/{request_id}` | 用户会话。 |
| POST | `/v1/keys` | 用户会话。 |
| GET | `/v1/keys` | 用户会话。 |
| PATCH | `/v1/keys/{key_id}` | 用户会话。 |
| DELETE | `/v1/keys/{key_id}` | 用户会话。 |
| POST | `/v1/tools/captcha/solve` | 用户会话。 |

### 9.3 公开解析接口

```http
POST /v1/captcha/solve
Authorization: Bearer cf_live_xxx
Content-Type: application/json
Idempotency-Key: UNIQUE_REQUEST_ID
```

```json
{
  "captcha_id": "captcha_xxx",
  "risk_type": "slide"
}
```

成功响应：

```json
{
  "success": true,
  "request_id": "req_01H...",
  "data": {
    "captcha_id": "captcha_xxx",
    "lot_number": "...",
    "captcha_output": "...",
    "pass_token": "...",
    "gen_time": "..."
  }
}
```

错误响应：

```json
{
  "success": false,
  "request_id": "req_01H...",
  "error": {
    "code": "QUOTA_EXHAUSTED",
    "message": "No remaining quota.",
    "retryable": false
  }
}
```

### 9.4 错误码

| HTTP | 错误码 | 配额变动 |
|---:|---|---|
| 401 | `API_KEY_INVALID` | 无。 |
| 403 | `SERVICE_UNAVAILABLE` | 无。 |
| 402 | `QUOTA_EXHAUSTED` | 无。 |
| 409 | `IDEMPOTENCY_IN_PROGRESS` | 无。 |
| 422 | `INVALID_REQUEST` | 无。 |
| 429 | `RATE_LIMITED` / `CONCURRENCY_LIMITED` | 无。 |
| 502 | `SOLVER_FAILED` | 退款。 |
| 504 | `SOLVER_TIMEOUT` | 退款。 |

### 9.5 管理端接口

管理端统一使用 `/admin/v1` 前缀，资源包括：`dashboard`、`cdk-batches`、`cdks`、`users`、`api-keys`、`calls`、`quota-ledger`、`settings`、`audit-logs`、`solver-health`。

## 10. 数据模型

### 10.1 `users`

`id uuid PK`、`status varchar`、`last_login_at timestamptz`、`created_at timestamptz`、`updated_at timestamptz`。用户身份由 `cdks.bound_user_id` 反查，不保存用户名或密码。

### 10.2 `cdk_batches`

`id uuid PK`、`name varchar UNIQUE`、`description text`、`default_quota bigint`、`activation_deadline timestamptz`、`service_duration_days integer`、`created_by uuid`、`created_at timestamptz`。

### 10.3 `cdks`

`id uuid PK`、`batch_id uuid FK`、`code_prefix varchar`、`code_hash bytea UNIQUE`、`status varchar`、`bound_user_id uuid UNIQUE FK users`、`activation_deadline timestamptz`、`expires_at timestamptz`、`quota_total bigint`、`quota_used bigint`、`quota_reserved bigint`、`quota_remaining bigint`、`activated_at timestamptz`、`last_used_at timestamptz`。

索引：`(batch_id, status)`、`expires_at`、`last_used_at`。所有额度字段非负；`quota_remaining` 是可受理额度，`quota_reserved` 是在途额度。

### 10.4 `api_keys`

`id uuid PK`、`user_id uuid FK users`、`name varchar`、`key_prefix varchar`、`key_last4 varchar(4)`、`key_hash bytea UNIQUE`、`status varchar`、`total_calls bigint`、`last_used_at timestamptz`、`created_at timestamptz`、`revoked_at timestamptz`。

索引：`(user_id, status)`、`last_used_at`。Key 删除采用软删除，历史关系不丢失。

### 10.5 `quota_ledger`

`id uuid PK`、`cdk_id uuid FK`、`user_id uuid FK`、`api_call_id uuid FK nullable`、`entry_type varchar`、`available_before bigint`、`delta_available bigint`、`available_after bigint`、`used_before bigint`、`used_after bigint`、`reserved_before bigint`、`reserved_after bigint`、`reason varchar`、`request_id varchar`、`actor_type varchar`、`actor_id uuid nullable`、`created_at timestamptz`。

索引：`(cdk_id, created_at)`、`(user_id, created_at)`、`request_id`；唯一约束 `(api_call_id, entry_type)`。该表仅追加，应用数据库角色不授予 `UPDATE`、`DELETE` 权限。

### 10.6 `api_calls`

`id uuid PK`、`request_id varchar UNIQUE`、`operation varchar`、`idempotency_key_hash bytea`、`user_id uuid FK`、`cdk_id uuid FK`、`api_key_id uuid FK`、`api_key_name_snapshot varchar`、`api_key_prefix_snapshot varchar`、`captcha_id varchar`、`risk_type varchar`、`status varchar`、`http_status smallint`、`error_code varchar nullable`、`error_summary varchar nullable`、`accepted_at timestamptz`、`completed_at timestamptz nullable`、`duration_ms integer nullable`、`quota_reserved boolean`、`quota_refunded boolean`、`client_ip_masked cidr`、`client_ip_hash bytea`、`user_agent varchar`。

唯一约束：`(user_id, operation, idempotency_key_hash)`。索引：`(user_id, accepted_at DESC)`、`(cdk_id, accepted_at DESC)`、`(api_key_id, accepted_at DESC)`、`(status, accepted_at DESC)`、`captcha_id`。

### 10.7 管理与配置表

- `admin_users`：管理员账户、密码哈希、角色、状态和最后登录时间；用户名唯一。
- `admin_audit_logs`：管理员、动作、目标、前后 JSON、原因、脱敏 IP 和时间。
- `system_settings`：`setting_key` 主键、`value_jsonb`、修改管理员和修改时间。
- `user_sessions`、`admin_sessions`：可撤销刷新会话，分别隔离用户与管理员认证。

## 11. 配额、并发和限流

### 11.1 配额账本

预扣：原子执行 `quota_remaining - 1` 与 `quota_reserved + 1`，并插入 `RESERVE` 流水。成功：`quota_reserved - 1` 与 `quota_used + 1`，插入 `CONFIRM` 流水。退款：`quota_reserved - 1` 与 `quota_remaining + 1`，插入 `REFUND` 流水。

更新条件必须包含 `quota_remaining > 0`，受影响行数为零时返回 `QUOTA_EXHAUSTED`，不访问底层服务。

### 11.2 Redis 控制

- Token Bucket：按 CDK 执行每分钟速率限制。
- Semaphore：按 CDK 执行并发限制；持有者在请求结束时释放，Redis TTL 作为故障回收兜底。
- 调用流程为：认证、幂等、速率、并发、额度预扣、下游调用、结算。
- `429` 响应包含 `Retry-After`；限流和并发拒绝必须写调用日志但不产生账本流水。

## 12. 安全、隐私与可观测性

- 管理员密码使用 Argon2id；用户仅使用 CDK 凭证换取短期会话 Cookie，Cookie 使用 `HttpOnly`、`Secure`、`SameSite`。
- API Key 与 CDK 通过服务端 pepper 计算 HMAC 哈希；原始值不写数据库、日志、审计或异常栈。
- 调用日志不保存 `X-Service-Key`、完整平台 API Key、底层上游 Token 或完整敏感解析结果。
- 客户端 IP 存储脱敏网段与不可逆关联哈希；User-Agent 限长。
- 底层服务请求使用严格连接、读取和总超时；平台向用户返回统一错误码，不返回下游错误正文。
- 每次请求产生 `request_id`，贯穿平台日志、调用记录、账本和下游请求头。
- 健康检查记录最近状态、延迟、连续失败数；失败告警不包含服务密钥。

## 13. 界面规范

- 面向开发者的紧凑工具界面，桌面优先，移动端保持关键操作可用。
- 卡片圆角不超过 8px；页面使用稳定的网格与表格布局。
- 使用 Lucide Icons；复制、刷新、筛选、导出、详情、禁用与删除使用图标按钮并提供悬停提示。
- Key、请求 ID 与 Captcha ID 使用等宽字体。
- 状态色固定：成功/可用为绿，处理中为蓝，失败/禁用为红，临期/额度耗尽/限流为琥珀，过期/删除/未激活为灰。
- 窄屏表格切换为摘要行和详情抽屉，不裁切关键状态或长标识符。

## 14. 验收标准

1. 浏览器请求与构建产物均不含底层服务地址和 `X-Service-Key`。
2. API Key 只在创建时完整返回一次，数据库与日志中没有 Key 明文。
3. 跨用户读取、修改 Key、日志、额度或 CDK 的请求一律被拒绝。
4. CDK 禁用、过期或额度耗尽后，关联 Key 的下一次解析请求立即失效。
5. 多个 Key 共享同一用户/CDK 额度池。
6. 并发受理时，剩余额度为 N，最多 N 个请求到达底层解析服务。
7. 同一幂等键只产生一次下游调用、一次预扣和一次终态账本结算。
8. 下游 5xx、超时与平台异常均恰好退款一次。
9. 调用日志可按用户、Key、Captcha ID、状态、时间和请求 ID 查询，且不泄露敏感凭据。
10. 管理员生成 CDK、调整额度、封禁用户和修改设置均有可检索审计记录。
11. 用户与管理端在桌面和移动端均可完成核心流程，筛选、分页、复制与详情状态完整。

## 15. V2 演进边界

- 支付订单、套餐、续期 CDK 与自动续费；
- 多验证码类型和多供应商执行适配器；
- 团队、子账号、项目级 Key 与 Key Scope；
- Webhook、异步批量任务和查询回调；
- 多节点队列、熔断、重试、区域容灾；
- 管理员 MFA、IP 白名单、Key IP 白名单与细粒度 RBAC。

V1 的解析调用封装为独立 `SolverGateway` 边界。新增服务类型时只增加适配器，不改变用户、Key、额度、账本和审计域模型。
