# 内网部署与本地开发

## Mac 与 Linux 共用远程浏览器模式（默认）

默认 `ORDER_BROWSER_MODE=docker`。Mac 运行 Docker Desktop；Linux 运行 Docker Engine。两边使用同一个浏览器镜像和 noVNC 链路。其他电脑只需浏览器，不需要安装 VNC 客户端。

Mac 先启动 Docker Desktop，在终端确认 `docker info` 可执行。停止旧的 Go 工作台进程（确认无同步任务），从项目根目录启动：

```sh
npm run start:remote
```

启动脚本检查 Docker、按需构建浏览器镜像、构建前端，然后启动 Go。镜像第一次构建需要下载依赖。之后仍可直接 `go run .`（要求镜像已构建）。Go 启动时检查 Docker 和镜像，失败会给出提示，不会静默退回本机模式。

本机访问 http://127.0.0.1:17381；其他电脑访问 `http://Mac内网IP:17381`（当前 Mac 为 http://192.168.1.104:17381）。未设置 ORDER_PUBLIC_ORIGIN 时支持本机与 192.168 网段地址；设置后只接受指定地址。其他电脑进入账户管理，点击“打开远程浏览器”→“打开远程桌面”，在网页里登录妙手。确认已验证后关闭远程桌面标签页，即可同步。

首次改用容器需要重新登录妙手：容器使用 `data/container-profiles/<账户目录>`，原来的 `data/profiles/<账户目录>` 不变，不跨 Mac/Linux 复用原生 Chrome 配置。历史订单、原始响应及账户记录不变。

如果先前已创建旧版浏览器容器，新版会提示重建。先停止对应账户同步、断开桌面，停止并移除旧容器后再打开远程浏览器；保留宿主机数据目录。不要手工覆盖或删除原生 Chrome profile。

仅需本机 Chrome 调试时仍可显式使用：

```sh
npm run start:local
```

本机模式不提供远程桌面，登录窗口仅出现在运行后端的电脑。

## Linux 服务器

Go 工作台在 Linux 宿主机运行，每个妙手账户一个独立容器，里面包含 Chromium、Playwright、Xvfb、x11vnc 和 noVNC。员工访问同一个工作台，通过账户的远程桌面完成登录。不同账户可以并行使用；同一账户只能有一个远程桌面连接，自动采集期间不允许连接桌面。

当前按可信内网使用，没有员工登录或账户权限分配。隔离单位是妙手账户，不是员工。员工可访问工作台里的所有账户。

### 构建

在项目根目录执行。构建机需要 Node.js 20.19+ 或 22.12+、Go 1.25.1+、C 编译器（SQLite 使用 CGO），以及 Docker。服务器运行用户需要能执行 Docker 命令，且有数据目录写权限。Go 程序与 Docker daemon 必须在同一台服务器；Mac 使用本机 Docker Desktop，不支持远程 Docker context。

```sh
npm ci
npm run build
go build -o order-processor .
docker build -f deploy/browser/Dockerfile -t order-processor-browser:1.63.0 .
```

镜像中的 Playwright 版本必须与 package-lock.json 一致。目前都固定为 1.63.0。服务器运行 Go 程序时不需要宿主机安装 Node.js（仅构建前端需要）。

### 启动

下面 IP 请替换为服务器内网地址。访问时使用与 ORDER_PUBLIC_ORIGIN 完全一致的协议、主机和端口。

```sh
export ORDER_BROWSER_MODE=docker
export ORDER_DATA_DIR=/srv/order-processor/data
export ORDER_LISTEN=0.0.0.0:17381
export ORDER_PUBLIC_ORIGIN=http://192.168.1.10:17381
./order-processor
```

先创建数据目录并确保运行用户能写入。从项目根目录启动，或将二进制和 dist 放在同一目录并从该目录启动。可选环境变量：ORDER_BROWSER_IMAGE（默认 order-processor-browser:1.63.0）、ORDER_BROWSER_PREFIX（默认 op-browser-）。同一数据库只启动一个 Go 工作台实例，桌面占用锁保存在该实例内存中。

### 使用流程

1. 添加妙手账户，点击“打开远程浏览器”，按需创建该账户容器。
2. 点击“打开远程桌面”，在远程 Chromium 中手动登录妙手并打开订单页，等待账户状态显示已验证。
3. 关闭远程桌面标签页，返回订单或产品页面启动同步。关闭标签页只断开远程控制，不会退出 Chromium。
4. 同步期间其他员工可以查看本地已入库数据，也可以采集其他账户。同一账户的远程桌面需等待同步结束或停止同步后再连接。
5. “关闭浏览器”关闭该账户 Chromium，保留登录目录；容器继续运行供下次打开使用。

远程桌面连接失败时，先查看是否有人正操作该账户，或该账户正在同步。登录过期、验证码仍由员工手动处理。

### 数据与重启

数据保存在宿主机 ORDER_DATA_DIR：orders.db、raw/<账户目录>、container-profiles/<账户目录>、runtime/<账户目录>、snapshots/<账户目录>（完整订单范围凭证）。每个容器仅挂载自己的四个账户目录，不挂载数据库或其他账户目录。数据库和业务记录沿用自增 ID；worker-token 是内部凭证，不是业务 ID。

Go 程序重启不关闭浏览器容器，运行中的采集可以继续，Go 恢复后继续解析原始响应。浏览器进程或容器重启会中断当前采集，任务恢复为失败并保留已采集进度，需要手动重新同步。账户登录状态尽量复用持久化目录；妙手会话过期仍需重新登录。

每个容器配置 2 CPU、3 GB 内存上限和 1 GB /dev/shm。没有自动回收策略；首次打开账户后容器会持续运行，请按账户并发数安排服务器容量。暂时不用时可以在服务器执行 docker stop 对应容器，下次点击“打开远程浏览器”会重新启动。

更新浏览器镜像后，已有容器不会自动替换。先停止该账户同步、关闭桌面，使用 docker stop 和 docker rm 删除对应 op-browser-<账户目录> 容器，再点击“打开远程浏览器”重建。不要删除宿主机账户数据目录。请备份整个数据目录；对 SQLite 在线备份应使用 SQLite backup，或停服务后备份数据库及其 WAL 文件。

### 本次订单范围修复如何生效

重启更新后的 Go 后端，会重放已有原始响应，修正此次待处理订单的源成员归属；已有完整单页存档为未出货 158、已出货 197。此步骤不需要重新向妙手请求，也不删除历史订单。订单页点击“查询”或重新进入后查看新计数。

多页完整范围凭证、以及“同步全部订单”结束后再校准两个待处理分组的流程属于浏览器 worker 代码。它通过 Dockerfile 的 `COPY browser` 放在镜像内，**只重启 Go、只重新启动旧容器，或再次执行按需构建的启动命令，都不会替换旧容器中的脚本**。升级步骤如下：

1. 等待所有相关账户同步结束，或在工作台停止同步并确认已停止；关闭这些账户的远程桌面标签页。
2. 在项目根目录显式重建镜像。如果设置了 `ORDER_BROWSER_IMAGE`，使用对应镜像标签：

   ```sh
   docker build -f deploy/browser/Dockerfile -t "${ORDER_BROWSER_IMAGE:-order-processor-browser:1.63.0}" .
   ```

3. 重启更新后的 Go 后端，使其使用包含 `snapshots` 挂载的容器配置。列出实际账户容器，按 `order-processor.account` 标签核对账户目录，不要把工作台数字账户 ID 当成容器名：

   ```sh
   docker ps -a --filter label=order-processor.account --format '{{.Names}}  {{.Label "order-processor.account"}}'
   ```

4. 对需要更新的账户逐个执行以下命令，将占位符替换为上一步确认的实际容器名称（默认前缀为 `op-browser-`，也可能配置了其他前缀）：

   ```sh
   docker stop <实际账户容器名称>
   docker rm <实际账户容器名称>
   ```

5. 返回该账户点击“打开远程浏览器”，由工作台使用新镜像和四个账户目录挂载重建容器。检查登录状态，必要时重新登录；断开远程桌面后再同步。新流程维持每页 200 条与 3～5 秒间隔，完整任务成功后保存范围凭证。

保留整个宿主机 `ORDER_DATA_DIR`，尤其数据库、`raw`、`container-profiles`、`runtime`、`snapshots`；不要删除目录、清理卷或移除其他账户容器。以上只是升级操作说明，本次开发未实际停止或删除容器。

### 请求头与网络

- 妙手请求由真实登录浏览器发起，Cookie、Origin、Referer、UA 和客户端提示由浏览器生成，不拼接其他账户 Cookie，也不伪装操作系统指纹。Linux 容器使用自身 Chromium 环境。
- 存档仅保存允许的请求头值；不保存 Cookie、Authorization 或未知自定义请求头的值。Referer 去掉查询串和片段。原始业务响应及请求体依旧保存以支持本地重放。
- Go 调用采集接口时重新构造请求，仅添加该容器的内部凭证，不转发员工请求头；凭证不出现在前端返回值或 URL 中，不跟随重定向。
- noVNC 转发只允许必要协议请求头，不转发员工 Cookie/Authorization。浏览器容器的 18080 和 6080 映射到宿主机 127.0.0.1 随机端口，员工通过 Go 工作台访问；5900 不映射。
- 工作台校验 Host、Origin、Sec-Fetch-Site，不信任 X-Forwarded-*。指定 ORDER_PUBLIC_ORIGIN 时只接受配置的公开地址，否则接受本机与 192.168 地址；不配置任意来源 CORS。
- 如公司已有 HTTPS 反向代理，可将 ORDER_PUBLIC_ORIGIN 配置为 HTTPS 地址，代理保留 Host 并支持 WebSocket Upgrade。当前 HTTP 配置不提供传输加密。

### 上线验证

自动测试覆盖账户容器归属、请求头隔离、来源校验、远程桌面占用阻止同步、任务进度恢复及原有分页采集。仍需在目标 Linux 上实际验证：两个账户分别登录、noVNC 连接与断开、200 条分页采集、采集时拒绝桌面连接、容器重启、Go 重启后继续入库。Mac 单元测试通过不等于 Linux 桌面链路已验证。

## 启动时提示 package-lock.json 不存在

`npm run start:remote` 现在会先运行 `scripts/prepare-lock.mjs`，再构建 Docker 镜像。它会检查锁文件、Playwright/npm 与浏览器镜像版本，以及跨平台可选依赖。锁缺失或只包含本机安装树信息时，在临时目录生成完整锁；保留已有锁中的版本固定信息，不把 Mac 的 node_modules 复制进 Linux 镜像。

此恢复步骤需要当前终端能访问 npm registry；成功后保存 package-lock.json，后续无需重复生成。不要通过删除 Dockerfile 的锁文件 COPY 或改用不受锁约束的安装来跳过。若提示 ENOTFOUND，请先检查终端的 npm registry 网络，然后重新执行同一条 `npm run start:remote`。

现有本机安装树恢复出的锁只用于保留版本，缺少 registry 校验信息/其他平台可选包时会被准备脚本识别并补全，不会当作完整锁直接开始 Docker 构建。
