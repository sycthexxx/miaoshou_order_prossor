# 订单数据工作台

公司内网部署的订单与采购工作台。支持网页采集和官方 API 两种数据接入方式，共用订单、出货分组、产品货源及采购汇总业务。

## 启动

需要 Go 1.25+、Node.js 22.12+、Docker（Mac 使用 Docker Desktop），以及编译 SQLite 驱动所需的 C 编译器（macOS Command Line Tools）。

```sh
npm install
npm run build
npm run start:remote
```

打开 http://127.0.0.1:17381；其他电脑可通过 Mac 的 192.168 内网地址访问。默认使用账户独立容器，点击“打开远程浏览器”后通过“打开远程桌面”登录妙手，验证后关闭桌面标签页再同步。首次从原生 Chrome 切换到容器需要重新登录。详细步骤见 [部署说明](deploy/README.md)。本机 Chrome 调试使用 `npm run start:local`。

开发前端可另运行 `npm run dev`，访问 Vite 显示的本地地址。


## 数据位置

- `data/orders.db`：SQLite 账户、订单、包裹、商品明细、中文规格和解析记录数据库（WAL 模式）。当前选择 SQLite 是为了单机启动零数据库服务依赖。
- `data/container-profiles/<accountId>/`：Mac/Linux 共用的容器浏览器账户目录，首次切换需登录。
- `data/profiles/<accountId>/`：账户独立 Chrome 用户目录，包含登录状态。关闭浏览器仍保留，重新打开可以复用，但有效期由妙手决定。
- `data/raw/<accountId>/<时间>-<序号>.json`：原始响应信封，含采集时间、接口路径、HTTP 状态、查询请求体、原始响应字节 Base64。文件先写临时文件并同步，再原子重命名。

响应正文不裁剪业务字段；不记录 Cookie、Authorization 或其他请求鉴权头。响应本身可能含订单隐私或临时签名，整个 data 目录均应作为公司敏感资料管理。原始记录当前不自动清理，需关注磁盘容量。退出程序后备份整个 data 目录，避免只复制活跃 WAL 数据库主文件。

离线解析原始响应（不访问妙手）：

```sh
node scripts/replay.mjs data/raw/<accountId>/<记录>.json
```

## 已纳入采集的接口

只监听专用浏览器中来自 `https://erp.91miaoshou.com` 的以下响应：

- `/api/order/package/render_list/searchOrderPackageList`
- `/api/order/package/getTranslatedOpOrderPackageList`
- `/api/order/purchase/purchase_item_source/getList`

网页接入通过专用浏览器执行正常页面查询并自动翻页，监听并保存响应。原始文件写入成功后才解析响应判断登录可用性；仅订单列表成功 JSON 才标记“订单读取已验证”。该状态是最近一次读取成功的证据，不代表持续有效或已确认实际账号身份。账户名称是本地标签，需要人工核对真实登录账户。

货源弹窗可能由妙手自动提交操作日志或保存选择。本程序不自动打开货源弹窗、不调用申请运单或采购写接口。

## 验证

```sh
go test ./...
go vet ./...
node --test internal/adapters/miaoshou/web/worker/archive.test.mjs
npm run check
npm run build
```

必须通过人工登录和实际订单响应验证后才能认定 Live PASS。编译/单元测试不替代真实登录验收。

## 网页请求环境

网页通道使用每账户专用 Chrome 的真实会话。当前只被动捕获正常页面请求，浏览器原生生成 Cookie、来源页、客户端提示和动态请求头，不硬编码或伪造签名。存档仅记录安全的请求环境摘要（浏览器 UA、客户端提示、前端版本、来源页路径、请求头名称），不保存鉴权头值。后续主动请求应先验证同一浏览器上下文的请求方式，不能假定复制 Cookie 即可复用。官方 API 单独实现官方签名。

## 订单入库与查询

启动时及每 5 秒从本地原始响应解析新增订单、包裹、商品明细和中文规格；不发送妙手请求。使用事务写入，成功后记录解析版本，重复重放不重复新增业务记录。旧响应不会覆盖采集时间更晚的数据。翻译可先于订单到达，通过包裹和商品外部 ID 关联。

网页默认显示第一个账户的本地订单，可按订单号、标题、规格或店铺搜索，每页 20 个包裹。页面数量是本地已采集数量，不是平台全部数量。后台服务更新后必须重启 `go run .`，然后刷新网页。

手动离线重放入库：`go run ./cmd/replay`。

所有业务表主键为 SQLite `INTEGER PRIMARY KEY AUTOINCREMENT`（64 位，等价于未来 MySQL 的 BIGINT 自增）。外部订单号和 SKU ID 以字符串保留，不转换为 JavaScript Number。去重使用账户和外部业务 ID 的唯一约束，不生成散列或 UUID 主键。存档不生成或检查哈希；旧存档中的校验字段忽略，仍可重放。

旧账户自动迁移为自增 ID。既有浏览器目录通过内部 storage_key 兼容保留，避免重新登录；新账户目录直接使用自增 ID。此次本地迁移前备份位于 `data/backups/before-integer-id-migration.db`。

## 全部订单同步

更新代码后重新运行 `go run .`，刷新工作台。账户管理中打开专用 Chrome 并完成订单读取验证，然后在订单记录页点击“同步所有订单”。

- 同步当前选中账户的「历史订单 → 全部」范围，重置筛选，从第 1 页逐页点击下一页，保留网站原有请求环境。不会触发妙手的“同步订单”、采购或运单提交操作。
- 每次翻页间隔 3～5 秒，不并发抓页。每页响应先写入本地原始存档；后台每 5 秒解析入库。同一账户重复启动时复用当前任务，可以停止。网络、登录、存档或分页异常会中断并显示原因；订单总数变化只提示并继续；已保存数据保留，再次同步按包裹唯一键更新，不累加数量。
- 同步任务进度在内存中，重启后需重新启动；原始响应及入库订单持久保存。网站没有快照分页，期间订单变化可能需要再次同步核对。当前功能不承诺覆盖妙手页面本身不可查询的数据。

验证：`GOCACHE=/tmp/order-go-build go test ./...`、`node --test internal/adapters/miaoshou/web/worker/*.test.mjs`、`npm run build`。

### 同步范围选择

订单记录页提供两个独立按钮：
- **同步全部订单**：历史订单的全部状态，全分页采集。
- **同步待处理订单**：订单处理的待处理状态，依次采集「未出货」「已出货」两个分组的全部分页。这里的「已出货」仍属于妙手待处理分类，不等同于顶层「已发货」。

进度显示本次范围、当前分组、分组页数/数量和累计去重包裹数。两种任务共用当前账户锁，不允许并发切换范围；需要先停止或等待当前任务结束。两种范围均保持 3～5 秒翻页间隔；同步较小范围不会删除已入库的其他状态订单。待处理任务的两组遍历已通过自动测试，尚待重启服务后完成真实全分页验收。

### 重复同步的入库规则

业务数据采用新增或更新：同一账户内以妙手订单 ID、包裹 ID，以及包裹内商品明细 ID 判重。已有记录更新字段并保留本地自增 ID；不存在才新增。全部同步和待处理同步共用这些唯一键，订单数量不会因重复同步累加。较早采集的响应不会覆盖较新的记录。原始响应每次独立留存，业务记录不因此重复。


### 动态订单与采集分页

采集通过妙手页面选择每页 200 条（已观察到可选 10/20/50/100/200/500），不改变本地订单展示的每页 20 条。翻页仍间隔 3～5 秒。总数增减会动态更新进度，不再判定为错误；按包裹 ID 去重并更新本地记录。遍历结束后回看首页；发现数量变化、新包裹或分页重叠时最多额外补扫一轮，避免持续有新单时无限循环。此方式不是数据库快照，无法承诺持续变化中一次采集零遗漏；页面明确提示后续同步继续补齐。未确认妙手新增订单是否通过 WebSocket 推送，不依赖该假设。

## 店铺产品与货源同步

左侧“店铺产品”提供 TikTok 产品同步入口，使用已登录的专用 Chrome 页面 `https://erp.91miaoshou.com/tiktok/item/item`，通过正常查询及分页观察 `/api/platform/tiktok/item/item/searchItemList`。每页 200 条，分页/切换状态间隔 3～5 秒；覆盖在售、下架、暂停销售、已删除，草稿不在此次范围。店铺范围沿用页面重置后的筛选，任务展示实际请求中的店铺 ID，不声称采集了其他未选中的店铺。与订单同步共用账户任务锁，支持停止。

产品响应先留存原始存档，再在同一事务入库。`shop_products.id` 为 64 位自增主键，业务唯一键为 `(account_id, platform, shop_id, platform_item_id)`，重复同步更新并保留本地 ID。订单明细同名 `platformItemId` 与该表关联；妙手内部 `itemId` 不是平台产品 ID，不能混用。平台外部 ID 始终保留字符串。

货源取产品列表 `sourceList`，包含 `source`、`sourceItemId`、`sourceItemUrl`，支持多个来源。订单卡片自动展示匹配产品的货源链接。区分未同步产品、来源字段未返回、明确空来源和已有来源，空来源是正常业务结果。货源列表不保证有厂家名称或 1688 SKU 对应关系，不据此猜测；当前保存整份产品 JSON，没有声称获取完整 SKU 明细。测试已覆盖账户/店铺隔离、空来源、更新保留 ID 和旧响应保护；完整实网全量产品同步仍待验证。


## 采购汇总

左侧“采购汇总”替代原手工商品归类功能，原页面、接口、规则实现与测试已移除，启动时清理旧规则表。汇总实时读取本地订单和产品，无需维护手工 SKU 归类。默认取 appPackageStatus=wait_seller_send、appPurchaseStatus=0、appPackageOperateStatus 为 wait_audit/wait_process 的明细，扣除 refundQuantity，默认排除赠品。按唯一货源（来源平台/站点/货源商品 ID）汇总同一商品的颜色与尺码，跨店铺合并；多候选货源或无货源单独待确认，不重复分配数量。当前未取得厂家稳定标识，因此不同货源商品不能擅自合并为同一厂家；显示厂家待确认。规格使用中文规格，缺失时保留原文，不推测 1688 SKU。可展开原始包裹和 SKU 明细核对。汇总不扣库存，也无法识别未录入妙手的手工采购。

### 采购卖家与货源指定

- 左侧「采购卖家」单独展示卖家、已指定货源及来源订单。「订单记录」可在商品货源下指定/更换/取消卖家；「采购汇总」只读展示，按卖家 → 货源产品 → 规格需求展示。一位卖家可对应多个产品和多个订单；指定关系按账户隔离，同货源跨店铺沿用。
- 卖家提取自 `/api/order/package/render_list/getOpOrderPackageIdAndOutboundInfoMap` 的 `purchaseOrders.*.purchaseOrderSeller`，不是店铺产品接口。出货信息原始响应先落盘，再由本地重放入库；订单分页等待该页包裹的出货信息齐全后继续，保持 200 条/页和 3～5 秒间隔。
- 当前接口未给出独立卖家 ID，按账户 + 采购平台 + 卖家账号文本去重，本地 ID 使用 SQLite 64 位整数自增。采购单号不是卖家 ID，也不是货源产品 ID。来源订单可能同时关联赠品，因此不自动推断货源归属，不覆盖人工指定。
- 升级后重启 `go run .`，重新打开账户浏览器，再同步全部订单以补充历史采购卖家；旧订单列表存档不包含这份独立接口数据，不能仅靠旧存档补全。编译前端使用 `npm run build`。

### 插件回传厂家名称的更新规则

插件通过原工作台同源调用 `POST /api/accounts/{id}/source-supplier/capture`，提交 `productId` 和 `name`。接口仅处理当前账户已同步的1688货源。现有厂家名称与本次名称去除首尾空白后精确相同时，保留关联及更新时间；名称不同时，复用当前账户同名1688厂家或新增自增ID厂家，并更新该商品的关联，包括原来人工指定的不同名关联。原厂家记录以及它关联的其他商品不会被删除或修改。

响应计数分别为 `assigned`（新关联）、`updated`（不同名改关联）、`retained`（同名保持），`created` 表示是否新增厂家，`supplierName`/`supplierId` 指示本次目标厂家。整个操作在一个事务中执行，失败全部回滚。此规则针对插件明确回传的厂家信息，不改变历史订单采集不推测货源归属的规则。

### 独立保存采购颜色 / SKU 映射

采购助手的映射保存到独立的 `procurement_mappings` 表：`id` 为 SQLite 64位兼容整数自增主键，业务唯一键为 `(account_id, source, product_id)`，当前来源固定1688。同一货源商品的多个 sourceSite 共用规则，不依赖卖家。保存映射不会修改 `shop_products`、订单原始 payload 或采集存档，产品/订单再次同步和本地 Replay 不会覆盖采购规则。

- `GET /api/accounts/{id}/procurement-mappings/{offerId}` 返回 `{revision,mapping}`；未保存时 `revision=0`，`mapping={version:2,colors:{},skus:{}}`。
- `PUT` 请求为 `{revision:<期望版本>,mapping:<完整规则>}`；成功后版本递增，包括重复保存相同内容。更新保留同一条映射记录的自增 ID。
- 规则结构：`colors` 保存原颜色标准化 key 到1688颜色的对应；`skus` 以原规格文本为 key，允许 `tail`（尺码等其余维度数组）、`exception`（单SKU例外）、`legacyAlias`（迁移旧映射）。DOM selector 只属于当前浏览器页面，不允许写入后端共享规则。
- 两台电脑同时编辑时，过期版本保存返回 HTTP 409 和当前 `{error,revision,mapping}`，不会覆盖其他人的更新；插件应重新读取后再操作。
- 读写均校验该1688商品属于当前账户已同步货源。若同步暂时移除了货源，原采购映射仍保留在独立表中，但接口拒绝对当前未知货源读写，重新同步出该货源后恢复可用。

新增后端接口及表迁移后需要重启 Go 服务。测试已覆盖映射保存、更新、清空、并发冲突、账户隔离、原数据不变、多次Replay不覆盖及重启持久化；不代表真实1688页面联调已完成。

订单待处理范围以妙手完整同步的页集合为准：单页完整响应直接记录范围，多页使用同步成功后保存在 `data/snapshots/<账户>/` 的完成凭证，并逐页校验原始响应、数量和订单 ID。未完成、取消、带额外筛选或不一致的页集合不会替换已有范围；离开当前范围的订单仍保留在历史数据中。采购汇总沿用该未出货范围，保留赠品选项与退款数量扣减。升级后启动时会从已保存原始响应重建范围，无需再次请求妙手；页面通过查询或重新进入显示更新后的结果。

采购汇总使用进程内缓存，按账户与“包含赠品”选项分别保存。普通读取和插件按 1688 商品编号查询都复用缓存；首次读取或服务重启后首次读取会计算一次。只有“刷新采购汇总”显式请求 `GET /api/accounts/{id}/procurement?gifts=0|1&refresh=1` 才更新对应缓存，同步订单、指定厂家和保存规格映射均不会自动使其失效。`GET /api/accounts/{id}/procurement/offers/{offerId}?gifts=0|1` 返回 `{found:true,snapshot:{...}}` 或 `{found:false,createdAt:...}`；快照仅包含当前账户该货源的商品标题、规格、数量及图片，时间为缓存生成的毫秒时间戳，不包含订单明细或买家信息。

### 官方 API 获取待处理订单

订单记录页新增「官方 API 同步待处理订单」，与「网页采集全部订单 / 网页采集待处理订单」并存。在「API 管理」中保存当前账户对应的名称、API ID（AppKey）、API Key（AppSecret），可选子账户 ID，再设为当前使用。凭证与当前选择保存在本地 SQLite；列表接口不返回 Key；点击编辑时读取完整凭证，表单直接显示 Key，可查看和修改，留空仍保留原值。同步时从服务端读取当前凭证，不再逐次输入。API 同步不需要打开登录浏览器；网页采集仍用于全部状态、产品货源、中文翻译和退款字段补充。

API 获取 `search_package_list(appPackageTab=waitProcess)` 的完整分页，再获取采购记录 `purchase_record/search_list` 的完整分页。以 `(platform, orderInfo.platformOrderSn)` 关联采购记录的 `opOrderPackageList`：存在 `relateStatus=normal` 的关联归「已出货」，否则归「未出货」。供应商的 `wait_send/has_send` 发货状态不用于这两组判断。订单号按原始 JSON 数字精度处理。遇到同订单多包裹等无法唯一关联、未知关联状态或独立赠品结构时，任务报错并提示使用网页采集。

两轮完整拉取的订单标识、商品数量/规格/赠品/退款相关字段和分组一致后，才在一个数据库事务内更新包裹、商品及两个分组。分页缺失、重复包裹、数据变动、取消或失败均不会提交部分结果；API 拉取期间如已有网页数据入库，也会放弃本轮提交，避免覆盖更新的数据。完整成功响应保存在 `data/openapi/<账户存储键>/<任务编号>/0001.json` 等文件，最终状态写入 `result.json`，文件权限为 0600。无需提交密钥即可查看当前任务进度及停止任务。

两种获取方式使用相同的包裹和商品唯一键、货源关联、规格翻译、卖家指定及采购汇总。API 的布尔型 `isGift` 归一为网页赠品标记；API 缺少的 `refundQuantity` 等字段按商品行 ID 保留原网页值，退款数据仍以该次网页采集时间为准；如官方订单退款时间发生变化，则将对应退款件数标为待补齐。新增商品行缺少退款数量且无退款记录时，采购汇总按退款 0 件计入需求；存在退款标记或退款时间但件数不明的商品仍列为「待补齐」，完成网页采集并刷新汇总后补齐。成功 API 同步会清除该账户采购缓存，下一次读取时重新计算。

本地回归样本（2026-11-17）：497 个包裹，未出货 305、已出货 192，分组成员完全一致。不含赠品：357 件、83 个货源分组、185 个规格；含赠品：662 件、87 个货源分组、191 个规格。API 合并前后逐货源、逐规格数量、订单来源行和卖家指定均一致。该结论针对已验证样本，实时退款变化仍需网页补充。

后端接口：`POST /api/accounts/{id}/sync-openapi` 无需请求体，使用当前账户已选中的 API 凭证；`GET /api/accounts/{id}/openapi-status` 查询进度；`POST /api/accounts/{id}/stop-openapi` 停止任务。当前 API 同步范围是待处理的两个采购分组，其他历史状态继续使用网页采集。

2026-11-17 15:15 的实际接入验收：先用保留的网页采集补齐新增商品的退款字段，再经新 API 同步入口完整拉取两轮并入库。最新 500 个包裹中，未出货 308、已出货 192，逐包裹成员与网页完成凭证一致；不含赠品 360 件（83 个货源分组、179 个规格），含赠品 668 件（87 个货源分组、183 个规格），两套汇总的规格、数量、订单明细及卖家指定均完全相同，待补齐商品为 0。验收记录位于本地 `data/openapi-validation/acceptance.json`，完整 API 响应位于对应 `data/openapi/` 任务目录。API 对 429、502、503、504 最多重试两次；仍失败则保留原有业务数据。

## 代码结构与扩展方式

项目只有一份业务系统；网页与 API 是可替换的数据接入通道。

```text
main.go                              启动与依赖组装
internal/
  server/                            HTTP 路由、页面接口
  application/ordersync/             同步任务：获取 → 提交 → 更新进度
  domain/
    orders/                          数据批次、订单合并、分组规则
    procurement/                     赠品、退款、货源和规格汇总
  adapters/miaoshou/
    openapi/                         官方签名、分页、关联字段转换、响应留存
    web/
      archive.go, scopes.go          网页响应和完整分页凭证解析
      worker/                        Node 浏览器采集进程
      browser/                       本机 / Docker 浏览器运行环境
  store/                             SQLite 读写、事务、归档重放、查询补充数据
  diagnostics/openapicompare/         手工核对工具，不参与业务运行
cmd/
  replay/                            重放本地网页存档
  openapi-compare/                    小批量官方 API 对照诊断
web/src/                             工作台前端
```

依赖方向：`server` 组装应用流程和适配器；`application` 依赖业务模型与接口；`adapters` 将外部格式转换为业务输入；`store` 实现持久化接口；`domain` 不依赖任何外部通道、HTTP 或数据库。持久化代码不再导入妙手 API 客户端，也不解析妙手接口 URL、签名或响应信封。

两种通道的时间模型不同，保留其自然形式：官方 API 由 `Source.Fetch` 拉取一个完整 `orders.Batch`，再通过 `Repository.ImportBatch` 提交；网页进程异步写存档，重放时由 `ArchiveDecoder` 转成 `orders.Archive`。两者共用 `orders.Record`、`orders.Scope`、`saveOrderRecord` 和业务合并规则。只有完整分组才能替换分组成员；单页订单可以先入库，未完成的多页采集不清空现有分组。

`orders.Record.ID` 是业务唯一标识，`Fields` 是兼容现有数据库和前端的订单读模型，保留扩展字段；它不是 HTTP 响应信封。此轮不重写已有字段名或数据库结构。适配器负责 `isGift`、采购关联等来源差异；业务层负责已有字段补充、退款数据有效性和采购计算。

增加数据来源时，实现拉取接口 `Source`，或对文件来源实现 `ArchiveDecoder`，输出上述统一模型并在入口组装即可。业务层不增加来源名称分支，不复制汇总代码。写操作使用独立业务接口：`application/packing.Submitter` 当前由网页适配器实现。官方采购运单写入试验仍仅由独立诊断命令调用，不参与手动提交代打包。

内部业务标识使用数据库自增 ID 和外部业务 ID，任务目录使用可读时间及序号。原始 JSON 不做 hash、SHA256、MD5 或 UUID 校验/编号。唯一保留的 SHA256 算法是妙手官方协议要求的 HMAC 请求签名。分页完整性、去重、事务、任务取消和退款件数处理属于数据正确性规则，继续保留。

诊断工具现在直接在项目根目录运行，例如：

```sh
go run ./cmd/openapi-compare --raw-dir data/raw/ACCOUNT_DIRECTORY --dry-run
```

原 `order_processor-openapi` 独立工程已迁出源码树，诊断代码与调研文档已合并。原工程全部文件、未提交修改及 Git 历史的可恢复备份为 `data/backups/legacy-openapi-project-20261117-153059.tar.gz`。重构前后的当前业务结果对照保存在 `data/refactor-validation/result.json`，按实际数据比较，不生成校验哈希。

### API 凭证管理

侧栏「API 管理」按当前账户维护多组名称、API ID 和 Key。支持添加、编辑、删除及切换当前使用的凭证，切换只影响后续任务，正在运行的任务继续使用启动时读取的凭证。删除当前凭证后需重新选择，系统不会自动换用另一组。

数据保存在 `data/orders.db` 的 `api_credentials` 表中，使用自增 ID；未增加哈希或额外加密机制。服务端凭证类型位于 `internal/domain/connections`，持久化位于 `internal/store/api_credentials.go`，HTTP 管理接口位于 `internal/server/api_credentials.go`。订单业务层和采集适配器不负责凭证管理。


### 手动提交代打包（网页适配器）

在「订单记录」点击包裹的「提交代打包」。当前支持不使用货代（`forwarderId=0`），录入 1688 采购信息或国内快递单号；勾选的商品共享本次采购单或快递单，选择普通商品时，同包裹赠品自动使用同一份提交信息。不同商品对应不同采购单时分次提交。通过账户管理中的妙手网页登录会话执行，不读取 API 管理中的 ID / Key。

本地入口：`POST /api/accounts/{accountId}/packages/{packageId}/packing`。业务参数定义在 `internal/domain/packing`；应用层校验所选订单商品行确实属于当前账户的包裹；网页适配器负责转换成 ERP 的嵌套表单：

```text
POST /api/order/purchase/manualRelateOrderPackagePurchaseOrders
purchaseOrderInfoList[0][opOrderPackageId]
purchaseOrderInfoList[0][opOrderItemIdAndPurchaseOrderInfoMap][订单商品行ID]...
```

这里使用 `opOrderItemId`，不是 `opOrderPackageItemId` 或 `goodsPurchaseOrderId`。采购信息模式的商品值是数组，快递单模式是对象。浏览器进程从当前加载的妙手代码识别其请求客户端，沿用原生表单编码、动态签名及登录会话。网页版本变化无法识别时，停止并提示更新适配器。

每次提交在 `data/runtime/<账户目录>/packing/<时间-序号>/` 保存业务转换后的 `request.json`、实际发送的 `request.form.txt`、完整 `response.txt` / `response.json` 和 `result.json`。没有收到响应时只记录请求与结果未知。请求失败不自动重发；成功判定同时检查 HTTP 状态、业务结果、成功/失败数量及逐项错误。

提交不会直接修改本地订单分组或采购汇总。成功后点击表单里的「同步待处理订单」，使用现有网页采集流程重新核对未出货、已出货；同步完成后点击订单列表「查询」。部分成功或结果未知时先去妙手核对，再决定后续操作。无需切换订单的官方 API 读取方式。

浏览器进程测试使用完全拦截的模拟网页与响应，不使用真实账号写入；Go 测试验证字段映射、商品归属、失败不重试、业务结果解析和不修改本地分组。更新浏览器代码后需重建镜像并更新已有容器，容器的 profiles/raw/runtime/snapshots 挂载目录须保留。

提交代打包前由 `application/packing.ShippingFirst` 先检查平台运单：妙手已有 `logisticsNo` 或本地已记录申请成功的包裹直接跳过；否则通过网页适配器逐包裹调用 `/api/order/package/logistics/batchApplyTrackingNo`（`opOrderPackageIds[0]`）。只有明确返回成功 1、失败 0 后才保存成功记录并提交采购信息。`getOpOrderPackageIdAndPrintWaybillLogListMap` 是打印记录查询，不是申请接口。

运单状态独立保存在 `shipping_labels`，以账户和包裹 ID 关联；任务重执行、代打包失败或程序重启不清除成功记录。申请中断或结果不明时只回查，不自动重发；待妙手有运单后可继续任务。物流商残留运单检查不通过时暂停，不绕过重复扣费提示。运单请求、表单及完整响应归档至 `data/runtime/<账户目录>/waybill/`。所有步骤继续共用原有串行提交锁。

### 采购汇总批量打包任务

采购汇总的每个已确认货源旁有「提交代打包」。弹窗的颜色、尺码筛选支持多选，规格以多选卡片展示，可同时选择不同颜色和尺码。所有选中规格共用上方填写的一份打包信息，保存为一个任务；也可用完整规格、SKU 或包裹号搜索，或展开逐条勾选订单。筛选变化保留已有勾选，“共用这份打包信息”区域始终展示全部已选规格，底部显示合计包裹、商品行和件数。选择当前货源的普通商品后，同包裹赠品自动使用同一份采购信息或快递信息，并在任务保存时一并固定下来；其他普通商品不会自动加入。采购汇总是否包含赠品不影响这个随单提交规则。不能可靠拆分颜色尺码的规格仍可用完整规格或 SKU 搜索。

保存时重新校验当前采购需求，冻结选中的包裹、订单商品行、规格、数量及填写的采购信息；同一包裹的多条商品合并成一次请求。任务和每个包裹结果存储于 SQLite 的 `packing_jobs` / `packing_job_items`，编号使用自增整数。关闭页面不影响执行，刷新可查看进度。

执行严格串行：所有任务共用一个执行器，数据库一次只允许领取一个包裹；网页提交入口另共用提交锁。上一包裹返回并保存结果后，再等待设定间隔（默认 5 秒，3～300 秒）才执行下一个；跨任务也保持间隔。每次网页请求只包含一个包裹。

失败、未发出或部分成功会暂停该任务。结果未知会阻止队列继续提交，须先在妙手核对；可明确标记已成功、确认未提交后重新排队，或跳过。暂停和取消不撤销已发出的请求，会等待当前包裹返回，取消只取消剩余项。重新排队后需点击「继续剩余包裹」。

重启时已完成项不再提交，未开始项保留；原本正在提交的项标记为结果未知并暂停，避免盲目重发。空闲等待中的任务在服务恢复后继续排队。网页登录失效或浏览器被占用时暂停，完成登录或关闭远程桌面后重新排队并继续。真实分组和采购汇总仍由网页采集更新，任务完成后点击「网页同步待处理订单」，采集结束后刷新采购汇总。

新增边界：`domain/packing` 准备固定的包裹请求；`application/packing.Queue` 管理串行执行；`store.PackingJobs` 持久化任务并原子领取、保存结果、恢复中断；`server/packing_jobs.go` 暴露任务列表、创建、明细与操作。启动入口把应用生命周期传给队列，继续复用网页适配器，不依赖官方 API 凭证。

### 独立测试：采购单更新平台运单信息

对应 [官方 api-479599777](https://s.apifox.cn/fd54e57e-9b98-4c34-bada-306221c39e68/api-479599777)，接口路径为 `/open/v1/ewm/goods_purchase_order/goods_purchase_order/operate/update_goods_purchase_order_outer_shipping`。2026-11-17 核对的文档以 `goodsPurchaseOrderId`（妙手采购单 ID）为必填字段；`purchaseOrderInfo` 和 `shippingId` 可选。采购单 ID 与包裹 ID、1688 订单号不同。该接口包含网页表单相似的采购字段，但是否能完成订单“提交代打包”、是否改变未出货/已出货分组，须实际回查，不能仅凭返回成功推定。

独立入口 `cmd/openapi-purchase-shipping` 不由后台服务启动，也不更新本地订单、分组或采购汇总。默认只预览 JSON，不访问数据库或网络；只有显式 `--submit` 才读取所选本地账户在 API 管理中设定的当前凭证并发送一次请求。数据库以只读模式打开，不运行迁移。写入不自动重试；连接中断或网关失败时保留结果未知状态，先核对远端采购单。

请求文件示例（先填入真实采购单 ID，其余字段按实际信息填写；不需要的可选字段应删除）：

```json
{
  "goodsPurchaseOrderId": "",
  "purchaseOrderInfo": {
    "purchasePlatform": "1688",
    "purchaseOrderSn": "",
    "purchaseOrderBuyer": "",
    "purchaseOrderSeller": "",
    "purchaseOrderPackages": [
      {
        "purchaseOrderLogisticsName": "",
        "purchaseOrderWaybillCode": ""
      }
    ]
  }
}
```

在项目根目录预览：

```sh
go run ./cmd/openapi-purchase-shipping --request data/purchase-shipping-request.json
```

资料核对完成后实际提交：

```sh
go run ./cmd/openapi-purchase-shipping --account 1 --request data/purchase-shipping-request.json --submit
```

`--db` 可指定数据库；`--response-dir` 可指定存档根目录。默认在 `data/openapi-purchase-shipping/<时间>/` 保存：

- `attempt.json`：接口、账户、凭证名称/编号与时间，不含 Key 或签名。
- `request.json`：与实际发送一致的请求 JSON，发送前落盘。
- `response.json`：完整原始响应，包括业务失败；非 JSON 响应写入 `response.txt`。
- `result.json`：HTTP 状态、API 是否明确报告成功及错误说明。没有收到响应时仍保存结果。

写接口接受文档中的无 `data` 成功信封；现有读取接口的解析和重试行为保持原样。测试使用注入的离线 HTTP transport，不会向真实妙手发送采购更新。

任务管理页面：左侧「任务管理」（`#/tasks`）集中查看当前账户的代打包任务，支持任务名称/编号搜索、状态筛选、包裹进度、创建/更新时间和执行明细。每 3 秒更新状态；沿用暂停、继续、取消剩余包裹和逐包裹异常处理。采购汇总保留任务入口，所有提交仍由同一后台队列串行执行。

订单详情中的单包裹提交同样记录到任务管理：先落库为正在提交，再调用网页适配器并保存完整结果与响应存档路径。服务中断后保留待核对记录，避免自动重复提交；单包裹与批量任务共用全局串行约束。

已完成的代打包任务支持「修改并重新执行」：读取原任务快照，自动补入同包裹赠品，按相同打包信息分组编辑采购信息、快递单、金额、备注和等待间隔；保存为新的串行任务，保留原任务及原响应。重执行按原任务包裹/商品范围校验，不要求订单仍属于待采购清单。预览和打开表单不会向妙手提交，点击「保存为新任务并执行」后才排队。
