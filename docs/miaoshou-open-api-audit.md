# 妙手官方 API 接入边界（2026-11-17）

本文保留官方 API 调研与实测过程。当前实现已合并回主工程：适配器位于 `internal/adapters/miaoshou/openapi`，诊断工具为 `cmd/openapi-compare`；业务分层见根目录 README。下文早期实验描述属于历史记录。

文档入口：https://s.apifox.cn/fd54e57e-9b98-4c34-bada-306221c39e68/

| 当前需求 | 文档证据 | 可覆盖范围及缺口 |
|---|---|---|
| 官方鉴权 | [快速接入](https://s.apifox.cn/fd54e57e-9b98-4c34-bada-306221c39e68/doc-8586023) | 正式根地址固定为 `https://openapi-erp.91miaoshou.com`。POST JSON；秒级 `x-timestamp`，`x-app-key`、`x-sign`，可选 `x-sub-account-id`。 |
| 包裹列表与详情 | [列表](https://s.apifox.cn/fd54e57e-9b98-4c34-bada-306221c39e68/api-524317133)、[详情](https://s.apifox.cn/fd54e57e-9b98-4c34-bada-306221c39e68/api-524317132) | 列表可见包裹 ID/号、状态、`skuSubName`、`qty`、SKU 图片及赠品信息。本次仅实现这两个读取接口。`platformOrderSns` 为逗号分隔字符串；时间筛选为字符串，未推测格式。 |
| 待处理未出货/已出货与中文规格 | 同上 | 当前所核对文档未见网页所用 `purchaseTab`、`appPurchaseStatus`、`skuSubCnName`、退款数量、`sourceList`。不能直接把官方列表当作网页待处理子组，也不能把 `qty` 原样当成本地 `quantity` 无差别替换。需要独立适配和业务校准。 |
| 退款数量扣减 | [售后接口](https://s.apifox.cn/fd54e57e-9b98-4c34-bada-306221c39e68/api-477739260) | 有售后 SKU 数量；与包裹商品的连接以及有效退款状态仍待确认，不能把所有售后申请数量直接扣除。 |
| TikTok 店铺产品 → 1688 货源 | 在线产品管理目录 | 当前可见在线产品目录为 Shopee 全球，TikTok 采集箱不等于 TikTok 在线店铺产品与采购货源关系。没有据此断言所有平台都没有接口；目前不足以替换现有网页 `sourceList` 关联。 |
| 厂家、采购记录、采购物流 | [采购记录一](https://s.apifox.cn/fd54e57e-9b98-4c34-bada-306221c39e68/api-523326861)、[采购记录二](https://s.apifox.cn/fd54e57e-9b98-4c34-bada-306221c39e68/api-523329034) | ERP 采购记录包含 `purchaseOrderSeller`、采购单号、订单关联和运单信息。稳定包裹主键关联、1688 账号全部采购记录覆盖尚未验证。 |
| 已知 1688 链接的 SKU 信息 | [1688 SKU](https://s.apifox.cn/fd54e57e-9b98-4c34-bada-306221c39e68/api-532434244) | 可按已知链接查询 SKU、规格图片及 ID；不提供 TikTok 产品到该链接的关系或厂家字段。未实现、未实调。 |
| 申请运单 | [单个运单](https://s.apifox.cn/fd54e57e-9b98-4c34-bada-306221c39e68/api-457255661)、[批量](https://s.apifox.cn/fd54e57e-9b98-4c34-bada-306221c39e68/api-523411121) | 文档确有以包裹 ID 申请单个运单，并有批量与面单目录。现有系统尚无写入实现，本次没有实现或调用。 |
| 采购汇总、规格映射和插件 | 本地业务 | 继续使用现有本地逻辑；不是官方 API 功能，不因新增客户端被覆盖。 |

## 客户端契约

包路径 `order_processor/internal/adapters/miaoshou/openapi`。调用方通过 `New(Credentials{...}, Options{...})` 提供凭证；客户端不读取配置文件、不落盘凭证、不记录请求或响应原文。不得把真实凭证写入源码或测试。

- `ListPackages(ctx, ListRequest)`：`/open/v1/order/package/fetch/search_package_list`，`page >= 1`、`pageSize` 在 10–200，返回 `data.orderPackageList,total,page,pageSize`。
- `GetPackage(ctx, positivePackageID)`：`/open/v1/order/package/fetch/get_package_info`，返回 `data.orderPackageInfo`。
- 以实际发送的 JSON 字节签名：HMAC-SHA256 的 key 为 AppSecret，消息为 `AppSecret + path + timestamp + AppKey + JSON字节 + AppSecret`，结果为小写十六进制。没有重编码后发送。
- 顶层 `result` 必须为 `success`；可选 `code` 如存在非空值，必须同为 `success`。不把失败消息或原始响应拼入错误。
- 不跟随重定向；请求根地址不可配置。可注入 `http.RoundTripper` 用于离线测试。
- 包裹与商品内容保持 `json.RawMessage`，避免平台长整型 ID 经 `float64` 舍入。后续解码应使用 `json.Decoder.UseNumber()`。

## 验证边界

自动化测试使用虚构凭证和模拟 HTTP transport，覆盖签名精确字节、秒级时间戳、可选子账户、列表/详情路径与参数、大 ID 保真、非法参数处理、重定向处理、错误脱敏、成功标记与载荷校验。接口实测结果见下文。

源码副本按文件类型和源码目录白名单复制，未复制 `data`、浏览器用户目录、原始响应、`.env`、`node_modules`、`dist` 或本机隐藏配置。基线 README 含历史说明，不能替代此处接口审计。

## 小批量真实只读对照工具

`cmd/openapi-compare` 是独立手动运行的 CLI，不接入旧服务、不写旧数据库或归档。必须通过 `--raw-dir` 明确选择一个本地账户目录；从最新有效“待处理/未出货”响应采样，默认 3 个包裹，最多 5 个。先读取原始响应并匹配账户目录，再按平台合并平台订单号，读取官方列表首个 200 条页面，最后按相同包裹 ID 逐个读取详情。请求之间间隔 1 秒，不自动重试或全量翻页。

在项目根目录执行（将占位符替换为本地账户归档目录名）：

```sh
go run ./cmd/openapi-compare --raw-dir data/raw/ACCOUNT_DIRECTORY --limit 3
```

运行时从 Mac/Linux `/dev/tty` 无回显读取 AppKey、AppSecret 和可选子账户 ID，不把它们写入文件、环境变量、命令参数或日志。凭证需对应所选本地账户。网络只请求客户端固定的正式域名，不接受代理/替代 API 根地址配置。终止信号恢复终端回显。

先检查本地采样、完全不请求网络：

```sh
go run ./cmd/openapi-compare --raw-dir data/raw/ACCOUNT_DIRECTORY --limit 3 --dry-run
```

输出为 JSON 行，包含本地样本时间、包裹 ID、列表/详情分别命中数、可比较字段数和差异。仅比较包裹/订单号、平台、店铺、状态、商品/SKU ID、规格、数量、赠品字段及图片等白名单；不打印买家字段或完整接口响应。数字与数字字符串归一，规格中文逗号和分隔空白归一；大 ID 不经过浮点数。商品兼容数组与以 ID 为键的对象映射，优先按唯一包裹商品行 ID、再按唯一订单商品行 ID 对齐；均无法对齐时，只允许同店铺且赠品类别明确相同的唯一平台产品 + SKU 组合回退匹配。歧义、赠品类别冲突、未知赠品类别的业务键回退均不自动匹配，不把数组位置当商品身份。`giftItems` 与 `items` 都参与，但没有推断未提供的赠品标记或退款数量。

未命中包裹返回 `found:false`，不作“不同商品”结论。列表超过首个 200 条时明确标记 incomplete；详情包裹 ID 缺失/不同不会比较。源快照时间与实际请求时刻不同可能造成状态或数量变化，差异不是自动覆盖依据。

### 商品对照修复

旧比较器只识别数组，实际本地网页订单 `items` 是对象映射时会读成零条，因此先前 `compared=9` 仅代表包裹/订单字段已比较，`additionalRemoteItems` 不能证明官方多商品。新报告包含两端 `items` / `giftItems` 原集合条数、无效条目数、匹配数、匹配方法、未匹配的最小商品身份，以及每个字段的 `equal` / `different` / `missingLocal` / `missingRemote` / `missingBoth` / `notComparedUnmatched` 状态。`compared` 仅累计两边都有值并真实比较的字段。商品身份仅输出商品行、产品/SKU ID、规格、数量、赠品类别，不输出标题或买家信息。

修复后首轮实测中，列表和详情各命中 3 个网页“未出货”包裹，6 条商品行均按 `opOrderPackageItemId` 匹配，其中普通商品和赠品各 3 条。再次运行同一命令会选该账户最新有效未出货档案，通过 `sourceCapturedAt` 核对采样时间。

### 补测采购与退款边界

在项目根目录运行以下一条命令，结果写入本地日志；凭证只从终端无回显输入，不进入管道或日志：

```sh
go run ./cmd/openapi-compare --raw-dir data/raw/ACCOUNT_DIRECTORY --cases 2>&1 | tee openapi-cases.log
```

`--cases` 为每类最多选一个有归档证据的代表：

- `hasPurchase`：请求明确处于待处理 `hasPurchase` 范围。
- `purchaseChanged`：同一包裹在两个不同时间存档中，明确记录 `appPurchaseStatus` 改变，或明确的 `waitPurchase` / `hasPurchase` 范围改变；输出前后时刻及值。
- `partialRefund`：商品 `refundQuantity` 或 `refundQty` 是有效整数，且严格大于 0、小于 `quantity`；优先明确非赠品，不将取消、退款标记或全额退货推断为部分退款。

没有证据的类别输出 `found:false`，不拿其他样本替代。历史证据时间与 `latestLocalAt` 分开；真实接口对照用该包裹最新本地表示，不能将历史状态当成当前状态。同一包裹覆盖多个类别时去重，仅请求一次详情，最多三个唯一包裹，仍只调用已有列表和详情读取接口，不新增或推测退款查询接口。原 `--limit 3` 未出货命令保持可用。

### 补测结果

`openapi-cases.log` 记录了两组真实对照。官方包裹列表和详情各命中两个包裹，4 条商品行均按 `opOrderPackageItemId` 匹配。SKU、规格、原始数量及归一后的赠品标记一致。两个接口在这批样本中均缺 `appPackageOperateStatus`、`appPurchaseStatus` 和商品行 `refundQuantity`。

| 样本 | 网页归档证据 |
|---|---|
| `2175547727` | 位于待处理的 `hasPurchase` 分组，`appPurchaseStatus=2`，包裹状态为 `wait_seller_send`。 |
| `2169274935` | 北京时间 11 月 16 日 21:11 到 11 月 17 日 13:16，从 `waitPurchase`（`0`）进入 `hasPurchase`（`2`）。两次 `appPackageStatus` 都是 `wait_seller_send`，`appPackageOperateStatus` 都是 `wait_process`。 |

`2169274935` 说明仅靠 `appPackageStatus` 无法复现网页采购分组。基础包裹及商品明细可继续做只读接入验证。采购汇总仍以现有数据为准，直到有证据能对齐采购分组和退款后的件数。

检查了本地网页原始归档及订单数据库，未发现同一商品行满足 `0 < refundQuantity < quantity` 的记录。下一步用一个真实样本验证 [官方售后接口](https://s.apifox.cn/fd54e57e-9b98-4c34-bada-306221c39e68/api-477739260) 的有效退款状态、商品行关联和退款件数。可同时试查 [官方采购记录接口](https://s.apifox.cn/fd54e57e-9b98-4c34-bada-306221c39e68/api-523326861)，用已知的 `0 → 2` 样本核对采购记录与包裹的关联。

### 完整响应与状态字段复核

重新展开官方列表和详情文档的响应示例，确认存在以下状态字段。下表路径相对于列表的 `data.orderPackageList[]` 或详情的 `data.orderPackageInfo`。

| 路径 | 文档含义 |
|---|---|
| `appPackageStatus` / `appPackageStatusText` | ERP 包裹状态及文本 |
| `platformPackageStatus` | 平台包裹状态 |
| `orderInfo.appOrderStatus` / `orderInfo.appOrderStatusText` | 妙手平台订单状态及文本 |
| `orderInfo.platformOrderStatus` | 平台订单状态 |
| `logisticsAgentProductInfo.status` | 物流渠道状态 |
| `opOrderPackageToPlatformLastMile.status` | 尾程物流渠道对象中的状态，业务映射待实测 |

列表请求支持 `appPackageStatus` 和 `appPackageTab`，后者文档明确为“妙手包裹tab(顶层tab)”。文档未列出它的可用枚举。原对比工具只检查固定字段，其 `missingRemote` 只证明指定路径缺失。其他字段、深层对象或 JSON 字符串中能否表达采购状态，需要实际响应验证。

`--inspect-states` 递归检查整个成功响应，包括包裹之外的数据、对象、数组及字符串内嵌 JSON（最多嵌套解码八层），输出字段路径、类型和状态候选值。收件人、联系方式、备注等子树只列结构。字段名命中仅用于发现候选，业务含义仍须核对。

`--response-dir` 保存完整成功响应、采集时间、接口路径和请求 JSON。每次响应写入独立文件，文件权限为 `0600`。使用以下命令采集本地已知两组样本，凭证从终端无回显输入。

```sh
go run ./cmd/openapi-compare --raw-dir data/raw/ACCOUNT_DIRECTORY --cases --inspect-states --response-dir ../data/openapi-responses
```

可通过 `--package-tab` / `--package-status` 传入官方列表筛选条件。筛选仅作用于列表；详情仍按已知包裹 ID 读取。列表未命中可能由筛选排除或分页引起。采购记录列表另有 `purchaseOrderStatus` 筛选，以及按 `platformOrderSn` / `appPackageNo` 检索的参数，可在确认包裹响应之后继续核对。

### 2026-11-17 完整 JSON 实测结果

北京时间约 14:34–14:39 调用正式接口，保存 22 份完整响应至主项目 `data/openapi-responses/`，逐层分析结果保存为该目录的 `state-analysis.json`。采集包含两个原补测包裹的列表和详情、15 个本地状态组合样本、7 类状态筛选、7 个顶层分组探测及 4 个采购子组对照。凭证仅用于请求签名。

| `appPackageStatus` | 实际中文文本 | 接口报告总数 | 本次检查行数 |
|---|---|---:|---:|
| `unpaid` | 未付款 | 89 | 10 |
| `pending` | 平台处理中 | 11 | 10 |
| `wait_seller_send` | 待发货 | 554 | 10 |
| `wait_receiver_confirm` | 已发货 | 551 | 10 |
| `finished` | 已完成 | 893 | 10 |
| `cancelled` | 已关闭 | 773 | 10 |
| `returned` | 已退款 | 3 | 3 |

七类筛选返回的样本行均符合请求状态。数量来自各请求时刻，分页验证覆盖各类首个页面。15 个历史状态组合样本全部命中，部分当前状态已变化。

顶层 `appPackageTab` 实测 `all` 返回总数 2874，`waitProcess` 返回 498，`waitShip` 返回 67，`waitReceiverConfirm` 返回 551，`closed` 返回 796。`waitProcess` 的首屏还包含 `pending`，不能仅用 `wait_seller_send` 自行重建该分组。`submitPlatform` 返回业务失败“没有符合条件的数据”，暂时无法区分空分组与取值不适用。无效值 `__invalid_probe__` 返回与 `all` 相同的总数和首屏，说明成功响应不能单独证明筛选生效。

实际响应同时包含 `orderInfo.appOrderStatus`、`orderInfo.appOrderStatusText`、`orderInfo.platformOrderStatus` 和 `platformPackageStatus`。例如包裹 `2029500625` 的包裹状态为 `returned`（已退款），嵌套订单状态为 `cancelled`，平台状态为 `CANCELLED`。各维度具有不同业务含义，应分别保留。当前样本的两个物流渠道 `status` / `extInfo` 都是空值，递归检查未发现字符串内嵌 JSON。

在这些完整响应的所有层级均未发现 `appPurchaseStatus`、`appPackageOperateStatus`、`refundQuantity`、`refundQty` 或 `purchaseTab` 字段。对固定订单集合分别省略 `purchaseTab`、传 `waitPurchase`、传 `hasPurchase`、传无效值，均返回完全相同的四个包裹，其中有历史采购状态 `0` 和 `2` 的样本。该实测表明此列表入口的 `purchaseTab` 未表现出子组筛选作用。

当前判断是官方列表已能读取及筛选七类生命周期状态，并可利用顶层 tab 获取部分处理分组。采购子组尚需通过采购记录接口或其他已验证关联补齐。`AWAITING_SHIPMENT` / `AWAITING_COLLECTION` 可以保留为平台状态，但还不足以与网页采购状态建立一一映射。

### 采购记录接口实测（2026-11-17）

已调用官方采购记录列表 `search_list` 与详情 `get_detail`，完整请求及响应保存到主项目 `data/openapi-purchase-responses/`，共 51 份。`purchase-analysis.json` 包含逐请求索引、状态筛选结果、订单关联、详情差异及全部字段路径。原始文件包括业务成功的空列表，便于复查阴性对照。

**可以关联订单，也能通过请求筛选确定采购单状态。** `purchaseOrderStatus` 在这批完整响应中缺失，但作为请求参数已验证有效，采集时应保留该参数对应的状态。

| 请求状态 | 含义 | 接口报告记录数 |
|---|---|---:|
| `wait_send` | 待出货 | 35 |
| `has_send` | 已出货 | 873 |
| `has_sign` | 已签收 | 176 |
| `finished` | 已完成 | 211 |
| `cancel` | 已取消 | 15 |

这五个状态来自已保存网页采购数据中的真实取值。各状态查询只检查首屏 10 条，数量为接口的 `total`，其实际类型为字符串。无效状态返回零条；五类总数之和为 1310，与无状态筛选的总数一致。

通过 `snType=platformOrderSn` 与 `snValue` 检索，8 个包裹样本中 6 个有采购记录，其返回的 `opOrderPackageList[]` 都包含目标平台订单号，另两个历史采购状态为 `0` 的样本返回零条。另以 `appPackageNo` 检索一个阳性、一个阴性样本，结果相符；不存在的订单号返回零条。

| 包裹 | 本次状态筛选命中 | 物流与关联证据 |
|---|---|---|
| `2175547727` | 仅 `has_send` | 关联命中，`gmtShip=2026-11-16 22:07:53`。 |
| `2169274935` | 仅 `has_send` | 关联命中，有采购物流单号，`gmtShip`、`gmtSign` 为空，物流订阅标记为 `0`。 |
| `2172663592` | 仅 `has_sign` | 关联命中，有发货与签收时间。 |
| `2172003747` | 仅 `wait_send` | 关联命中，物流单号为空，采购物流数组为空。 |

前三个样本交叉查询 `wait_send`、`has_send`、`has_sign`、`finished`，第四个另查 `cancel`。结果互斥。`gmtShip` 为空的 `2169274935` 仍属于 `has_send`，因此应优先使用服务端状态筛选。

当前网页分组的等价性仍需校准。`2172003747` 在历史网页快照中 `appPurchaseStatus=2`，本次官方采购查询只属于 `wait_send`。两次采样时刻不同，网页包裹采购状态与采购单状态也可能有不同汇总规则。零采购记录、取消采购单、多张采购单以及部分商品出货应分别处理，直到可以与同一时刻的网页分组核对。

关联路径为采购记录 `opOrderPackageList[].platform + platformOrderSn` 对接订单 `platform + orderInfo.platformOrderSn`。返回中有 `shopName`，但缺少 `shopId`、`opOrderPackageId`、`opOrderPackageItemId` 和 `platformSkuId`。因此目前验证到平台订单层级；拆包、多店铺重复编号和逐商品行数量分配需要额外校验。

存在多对多与重复来源。无条件列表首屏 50 条只有 25 个不同采购单号，每个单号各有 `1688` 和 `1688妙手` 两条来源记录，订单关联可不同。详情查询在若干样本中返回 `1688妙手` 的空关联列表，而列表中的 `1688` 记录有关联；分别只传采购单号、只传物流单号、同时传两者，结果一致。采集应保留来源和列表关联，详情可以补物流信息；去重和合并关联时需要同时核对采购单号、来源和物流信息，防止重复累计采购数量。

样本中的一张采购记录关联 24 个平台订单，另一张关联 56 个。关联状态 `relateStatus=normal` 表示关联信息，采购出货状态来自请求筛选。完整 JSON 的字段结构基本符合官方响应示例，但空值、重复来源及详情关联缺失以本次实测为准。

### 同时间网页复核与整组复现（2026-11-17）

已在登录后的网页中查询 `2172003747`，并记录其官方接口请求前后的网页响应。新证据确认两个状态层级同时存在。网页待处理的“未出货”筛选返回零条，“已出货”返回该包裹，`appPurchaseStatus=2`；同一页面的采购状态明确显示“待发货”，网页采购关联对象 `purchaseOrders.*.purchaseOrderStatus=wait_send`。官方采购接口也只在 `purchaseOrderStatus=wait_send` 下返回采购单 `5127599486641126324`。两端采购单状态的字段名与取值一致。

因此，先前把网页 `appPurchaseStatus=2` 与采购单 `wait_send` 视为相互矛盾，混淆了包裹采购关联状态和供应商发货状态。官方查询前后网页均保持上述值，本次差异可由状态层级解释。

进一步完整采集网页未出货两页（200+105）、已出货一页（192），以及官方待处理列表三页（200+200+97）和官方采购记录三页（500+500+310）。共 497 个待处理包裹、1310 条采购来源记录。官方待处理与网页包裹 ID 集合完全一致；复查官方列表第一页，ID 与顺序保持一致。

采用以下规则在本地重建当前分组。

1. 从官方 `appPackageTab=waitProcess` 获取全部待处理包裹。
2. 以 `platform + orderInfo.platformOrderSn` 关联采购列表 `opOrderPackageList[].platform + platformOrderSn`。
3. 存在 `relateStatus=normal` 的采购关联时，归入 `hasPurchase`；无有效关联时归入 `waitPurchase`。
4. 将供应商采购单的 `wait_send`、`has_send`、`has_sign` 等单独保留为采购状态。

| 分组 | 最新网页 | 官方数据重建 | 逐包差异 |
|---|---:|---:|---:|
| 未出货 `waitPurchase` | 305 | 305 | 0 |
| 已出货 `hasPurchase` | 192 | 192 | 0 |
| 合计 | 497 | 497 | 0 |

192 个已出货包裹均有正常采购关联，其中网页 190 个包裹有一张采购单，两个包裹有两张采购单。305 个未出货包裹均无采购关联。部分已出货包裹还存在 `relateStatus=cancel` 的历史关联，按 `normal` 过滤后仍全部匹配。

此规则在本账户当前完整待处理集合上得到验证。当前所有平台订单键唯一，网页状态仅为 `0`、`2`，且 `isManualStockout=false`。部分采购状态 `1`、拆包、多店铺重复订单号、库存与手工出库的边界尚需独立验证。实际接入时，分页不完整或订单键歧义应保留待核对状态。

全部证据保存在主项目 `data/state-recheck-2172003747/`。`group-reconstruction-analysis.json` 包含 27 份响应索引、采集时间、两端分页计数、逐包结果、关联采购单号及目标订单前后复核结果。
