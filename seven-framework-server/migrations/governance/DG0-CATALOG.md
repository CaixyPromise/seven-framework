# DG0 Go 数据库存储治理目录

**状态：DG0 已冻结；B1、B2、B3 已在两个专用隔离库完成原地改名验收。**

本目录是 Go 后端物理表改名的审查基线。它和下列机器可读文件共同构成
DG0 的完整目录，三者缺一不可：

- [`table_registry.csv`](table_registry.csv)：87 张实际物理表的逐表
  `现名 -> 目标名`、所属批次和改名前后验证状态；
- [`operational_sql_manifest.csv`](operational_sql_manifest.csv)：`cmd/`、
  任务/监听器以及 `scripts/` / `script/` 中每个实际执行 SQL 的运行时契约；
- [`future_table_registry.csv`](future_table_registry.csv)：后续新建物理表的
  显式登记入口；当前为空，避免把“将来可能创建”伪装成已有表；
- 本文件：拥有者、关系责任、批次次序、方言与隔离验证门禁。

`table_registry.csv` 当前有 87 条数据行：42 张 B1/B2/B3 表已完成下划线改名，
45 张已经符合命名。目标名无冲突。列名、Go 字段、JSON 属性和公开 API
**保持不动**，不在本计划中改名。

## 1. 全量目录与来源

| 域 | 表数 | 表目录 / 命名结果 | 迁移与运行时拥有者 |
| --- | ---: | --- | --- |
| Docker | 6 | 均已是下划线 | `migrations/mysql/20260430090000_docker_starter.sql`、`20260508090000_docker_compose_project.sql`、`20260730150000_docker_operation_integrity.sql`；`internal/infrastructure/docker/` |
| 外部身份 | 6 | `sysExternal* -> sys_external_*` | `20260621100000_external_oauth_consumer_v1.sql`；`internal/app/external_login/infrastructure/` |
| 联邦 | 2 | `sysFederated* -> sys_federated_*` | `20260712120000_federated_hub_node_v1.sql`；`internal/app/hub_control/infrastructure/` |
| 通知 | 20 | `sysNotification* -> sys_notification_*` | `20260625100000_notification_center_v1.sql` 至 `20260727170000_notification_delivery_diagnostics.sql`；`internal/app/notification/infrastructure/` |
| 平台 | 5 | `sysPlatform* -> sys_platform_*` | `20260622150000_platform_management_v1.sql`；`internal/app/platform/infrastructure/` |
| SSO | 9 | `sysSso* -> sys_sso_*` | `20260618100000_sso_provider_v1.sql`；`internal/app/sso/infrastructure/` |
| 配置 | 4 | 均已是下划线 | `20260528102500_system_config_schema.sql`、`20260612190000_config_scope.sql`；`internal/app/system/config/` |
| 字典 | 2 | 均已是下划线 | `20260719100000_system_dict_baseline.sql`；`internal/app/system/dict/infrastructure/` |
| 文件 | 10 | 均已是下划线 | `20260422000000_baseline.sql`、`20260730120000_file_asset_credentials.sql`；`internal/app/file/infrastructure/` |
| 授权 / RBAC | 11 | 均已是下划线 | `20260424090000_system_user_admin.sql`、`20260719120000_role_grant_revision.sql`；`internal/app/authorization/`、`internal/app/system/user/` |
| 组织 | 6 | 均已是下划线 | baseline；`internal/app/system/user/` |
| 用户 | 2 | 均已是下划线 | baseline；`internal/app/system/user/`、`internal/app/credential/` |
| 消息 / Outbox | 2 | 均已是下划线 | baseline；`internal/infrastructure/messaging/outbox/` |
| 安全 | 1 | 已是下划线 | baseline |
| 审计 | 1 | 已是下划线 | baseline；`internal/app/system/admin/` |

MySQL 历史链和 PostgreSQL 最终物理集合各为 87 张表。PostgreSQL 的 clean
install 必须先应用 `migrations/postgres-baseline/20260719110000_clean_install_baseline.sql`，
随后继续 `migrations/postgres/` 的增量；只验证其中一个路径不构成通过。

下列三张表当前只由迁移拥有、未发现非测试 Go 运行时引用：

```text
sysSsoIssuerKey
sys_file_integrity_audit
sys_storage_alert_log
```

它们仍在逐表 registry 中，不能因暂时没有运行时引用而绕过治理或静默删除。

## 2. 表名与字段的不可变契约

1. 每张 Go 拥有表的现名和目标名必须以 `table_registry.csv` 为准；表名采用
   lower_snake_case。
2. 本轮禁止 `RENAME COLUMN`，也不改变字段类型、默认值、可空性、主键、唯一
   约束、索引、Go 字段、JSON 和 API 契约。每一个后续表改名批次都要在两库比较
   这些签名。
3. PostgreSQL 保留的驼峰字段必须通过仓库局部的、受 allowlist 约束的方言 renderer
   或静态 SQL 引号处理；`sqlx.Rebind` 只解决占位符，不能替代标识符渲染。
4. 禁止通用字符串替换改写 SQL 标识符，运行时输入只能作为绑定值，不能成为表名、
   字段名或 SQL 片段。

## 3. SQL、任务、脚本与种子数据

生产原始 SQL 主要位于 repository 和 outbox store；具体运行期入口由
`operational_sql_manifest.csv` 锁定。当前清单覆盖 4 个 Go command 与 8 个可执行
审计/fixture 脚本。历史 audit fixture 均明确为 `mysql-only`，不能被误当成
PostgreSQL 生产路径。

自动扫描范围如下：

```text
cmd/**/*.go
internal/**/job/**/*.go
internal/**/listener/**/*.go
scripts/**/*.{py,js,mjs,sh,sql}
script/**/*.{py,js,mjs,sh,sql}
```

不包含仅断言 SQL 文本且不调用数据库进程的测试脚本。若一个测试脚本会执行数据库
客户端，它仍必须进入清单。以后新增的 SQL-bearing source 没有 manifest 行、方言
声明或 registry 支持时，测试必须失败。

迁移文件本身是 schema/seed 的唯一权威来源；操作 fixture/seed 不得私自绕过
registry。`sqlc` 目录当前没有额外业务 schema 或生成 SQL 表拥有者。

## 4. 禁用外键后的业务责任

今后 Go 拥有的 MySQL 和 PostgreSQL 增量迁移禁止 `FOREIGN KEY`，也禁止内联
`REFERENCES`。这不移除主键、唯一、非空或安全的标量 check 约束。

历史发现的 Docker 外键为：

```text
docker_operation_event.operationId -> docker_operation.id
```

其后续 forward migration 已移除外键。后续任何关联都必须由领域/应用层明确承担：

| 行为 | 必须由业务代码承担的责任 |
| --- | --- |
| 创建或替换引用 | 在同一事务内验证目标存在、范围、状态与调用者授权；使用条件写入、版本或幂等键处理竞争。 |
| 删除、停用或替换父项 | 先检查子引用，定义 child-first、显式级联或拒绝策略；不得依赖数据库级联。 |
| 跨表异步副作用 | 与业务提交同事务写 Outbox，并以幂等消费者和可恢复重试处理。 |
| 异常和悬空引用 | 有界扫描、先诊断后修复、二次确认、审计和限速；不能直接批量删除。 |

配置、字典与文件引用的额外保护规则：

- 删除配置组先检查配置；配置变更、审计和缓存失效保持同一提交边界；
- 字典项写入校验父级及唯一性；删除字典类型先检查或显式处理子项；
- 上传只返回 `fileId`；业务提交成功时才创建/替换 `sys_file_reference`；
  `fileId` 不是授权，服务端仍验证范围、上传归属/转移、状态、类型、内容安全和
  当前配置权限。

## 5. 迁移批次与冻结顺序

| 顺序 | 批次 | 范围 | DG0 决定 |
| ---: | --- | --- | --- |
| 0 | DG0 | 目录、规则、静态扫描、隔离验证合同 | 本 checkpoint；不改表、不迁数据。 |
| 1 | B1 | SSO 9 表 + 平台 5 表 | 已完成：两个隔离库均以既有仓储完成改前写入、原地改名、改后读/改/新建与旧名消失确认。 |
| 2 | B2 | 外部身份 6 表 + 联邦 2 表 | 已完成：两个隔离库均由 external-login、hub-control 和 platform 仓储完成改前写入、原地改名、改后读/改/新建与旧名消失确认；同时补齐 PostgreSQL 标识符与 upsert 方言。 |
| 3 | B3 | 通知 20 表 | 已完成：两个隔离库均通过通知 Client、仓储和受控验收命令完成改前写入、原地改名、改后读取/更新/新建、旧名消失及前向恢复拒绝 Down 验证；Outbox、收件箱、投递、快照及诊断未被分裂。 |
| 4 | Protected | 配置 4 + 字典 2 + 文件 10 | 均已是下划线，**不改表名**；作为 DC1/DC2A/DC3A 回归保护批，DC2B/DC3B 继续 Pending。 |
| 5 | No-op | Docker、RBAC、组织、用户、消息、安全、审计等已合规表 | 不做名称迁移；持续纳入 future migration/SQL 扫描。 |

B1/B2/B3 的工作方式一致：不是先选一张“试点表”，也不复制/双写数据。每个批次
在隔离库中先由当前 repository、Facade 或受控 HTTP 调用创建最小 fixture；没有可用
路径时才补一个受控 fixture。随后执行两库的原地 `RENAME TABLE` / `ALTER TABLE RENAME`
迁移，替换该批所有 repository、cmd、job、listener、脚本和 seed 中的旧表名。改后
必须用**同一调用**读取改前记录、执行支持的更新并创建一条新记录；只用数据库直查
确认新表名、旧表名消失、行数和主键不变。

`sysExternalManagedProviderCommand` 已随 B2 和 external-login 的 PostgreSQL renderer
一起完成，未留下只改表名、不修复方言的半成品路径。

## 6. 原地改名与回退窗口

所有批次使用前向恢复，不使用 Goose `Down` 回退真实数据：

1. **改前**：用当前业务调用造数并保存稳定 ID、可观察字段和预期行为；没有现成
   记录时才造最小 fixture。
2. **迁移与代码**：在同一受控发布中原地改名，并把所有运行时旧名替换为新名；
   迁移完成后再启动新代码，不允许旧代码与新表并存运行。
3. **改后**：用相同调用读取改前记录、更新和新建记录；再以数据库直查确认新物理
   表承接原数据、旧物理名不存在。`rg` 旧表名只允许命中历史 migration、registry、
   审计证据和明确的负向测试。

原地 rename 不产生回填、双写、主键同步或旧新表 union。失败时保持当前
schema，修复引用后前向发布；不使用 Goose `Down`。发布前保留数据库备份/恢复点和
上一版应用构件，直到本批次的改后调用与回归验证通过 checkpoint。

## 7. 仅允许的隔离库与验证命令

DG0 本身只运行静态检查。后续真迁移/CRUD 验证只能连接以下精确库名：

```text
MySQL:    seven_database_governance_mysql
Postgres: seven_database_governance_pg
```

禁止连接 `lovely_seven`、开发库、生产库或带后缀的临时库。每次运行前由
`AssertConnectedDatabase` 校验名称和方言。

在只指向上述隔离库的 profile 中，按顺序执行：

```bash
# 不运行时只记录为 NOT RUN；不要改用开发库。
SEVEN_PROFILE=<isolated-profile> make migrate-status
SEVEN_PROFILE=<isolated-profile> make migrate-up

# 两个数据库各执行 clean、upgrade、forward-recovery 三次。
SEVEN_PROFILE=<isolated-profile> \
DG1_DATABASE_GOVERNANCE_ACCEPTANCE=clean \
go test ./internal/infrastructure/datasource/governance -run '^TestDG1MigrationHistory$' -count=1

SEVEN_PROFILE=<isolated-profile> \
DG1_DATABASE_GOVERNANCE_ACCEPTANCE=upgrade \
go test ./internal/infrastructure/datasource/governance -run '^TestDG1MigrationHistory$' -count=1

SEVEN_PROFILE=<isolated-profile> \
DG1_DATABASE_GOVERNANCE_ACCEPTANCE=forward-recovery \
go test ./internal/infrastructure/datasource/governance -run '^TestDG1MigrationHistory$' -count=1
```

PostgreSQL 的 clean 路径由测试计划先应用 clean baseline、再应用增量；不得把
`make migrate-up` 当成完整替代。真实批次追加改名前后相同业务调用、fixture 直接
校验、受影响 package 测试、`go test -race` 和对应 UI 合同检查。

## 8. 自动门禁

`internal/infrastructure/datasource/governance/dg0_static_contract_test.go` 强制：

- 20260730160000 之后的新 MySQL/PostgreSQL migration 只能创建/重命名为
  lower_snake_case 表，并且目标必须已登记在既有或 future registry；
- 拒绝 `FOREIGN KEY` 和内联 `REFERENCES`；
- 拒绝 `RENAME COLUMN`，防止表名治理暗中变成字段改名；
- 历史混合表名必须出现在 registry；
- `cmd/`、job/listener、`scripts/`、`script/` 的实际 SQL 必须有 manifest、方言
  合同，并只引用 registry 中的物理表。

配合已有 `query_width_test.go`，Go 逻辑读取不得借 CTE、子查询或 view 绕开三张
物理表实例的查询宽度限制。脚本的实际表引用由 manifest 暴露；若脚本成为生产读取
路径，必须在对应批次把查询宽度和结果上限纳入审查，不能把它当成 repository 扫描
之外的豁免。

## 9. DG0 的明确非目标

- 不在开发、生产或业务库执行真实表 rename、复制、数据回填或 migration；B1/B2/B3 仅在
  两个专用隔离库执行并已通过；
- 不执行 Goose `Down`；
- 不新增外键，也不删除主键、唯一、非空或安全 scalar check；
- 不实现缓存、索引重写、DC2B/DC3B 或通知功能；
- 不声称生产流量、索引收益或缓存命中已经验证。
