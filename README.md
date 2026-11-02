# 小麦校园 · 今日校园查寝助手

用浏览器管理今日校园（CPDaily / Campusphere）的**宿舍查寝**任务：多账号、扫码绑定设备、自动获取签到点、服务端定时执行、照片管理与执行记录。

> 本项目**只处理查寝任务**，二维码签到类任务不会执行。

后端是单个 Go 二进制（内嵌前端与 SQLite），可直接跑在 Linux amd64 服务器，也可在 Windows / macOS 本机双击运行；全部界面能力都通过 REST API 暴露，方便二次开发。

> 本项目是 [qingjin-cpdaily](https://github.com/3238614968/qingjin-cpdaily) 界面形态的开源复刻，协议实现参考
> [CpdailyHelper](https://github.com/YVFENGXS/CpdailyHelper)、
> [jrxyzuixingnx](https://github.com/maserpoassr/jrxyzuixingnx)、
> [campus-device-id](https://github.com/2257929377/campus-device-id)。
> 请在学校及平台允许范围内，使用本人或已获授权的账号，并如实确认任务信息。

---

## 功能

| 模块 | 说明 |
| --- | --- |
| 工作台 | 今日执行统计、账号概览、最近执行、额度、站内公告 |
| 学生账号 | 多账号绑定、账密登录 / Cookie 导入、扫码绑定设备信息、**自动获取学校签到点**、签到照片 |
| 任务执行 | **仅查寝**：拉取待办、位置校验、照片上传、加密提交 |
| 自动计划 | 跟随学校下发时间或自定义时段，按星期执行，服务端调度、关页面继续跑 |
| 执行记录 | 成功 / 失败 / 结果待核对，附带坐标、签到点距离、服务端返回 |
| 额度与卡密 | 每次执行扣减额度，卡密兑换 |
| 账号与安全 | 修改密码、深浅色主题、开放 API 说明 |
| 设备 ID 页 | `/device.html`，用今日校园 App 扫一扫把设备 ID 回填到账号 |

## 技术栈

- **后端**：Go 1.24+，标准库 `net/http`（1.22+ 路由模式），`modernc.org/sqlite`（纯 Go，无需 CGO）
- **协议层**：`internal/cpdaily`（自研实现，无第三方 SDK）
- **前端**：原生 ES Module + 手写 CSS，无构建步骤；通过 `go:embed` 打进二进制
- **存储**：SQLite（WAL），账号密码用 AES-256-GCM 加密，平台密码用 scrypt 哈希

## 快速开始

```bash
# 1. 编译（Windows 生成 qingjin.exe）
go build -o bin/qingjin ./cmd/qingjin

# 2. 运行
./bin/qingjin -addr :8090 -data ./data

# 3. 浏览器打开 http://localhost:8090 ，注册第一个账号即可使用
```

### 部署到 Linux 服务器（amd64）

```bash
# 在任意机器上交叉编译，产物为静态单文件
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o qingjin ./cmd/qingjin

# 上传后运行（首次运行需设置管理员，避免公网被人抢先注册）
QINGJIN_ADMIN_USER=admin QINGJIN_ADMIN_PASS='换成你的强密码' ./qingjin -addr :8090 -data /var/lib/qingjin
```

也可用 Docker：

```bash
docker build -t qingjin .
docker run -d -p 8090:8090 -v $PWD/data:/data \
  -e QINGJIN_ADMIN_USER=admin -e QINGJIN_ADMIN_PASS='换成你的强密码' qingjin
```

## 配置

全部通过环境变量或命令行参数：

| 变量 | 命令行 | 默认 | 说明 |
| --- | --- | --- | --- |
| `QINGJIN_ADDR` | `-addr` | `:8090` | 监听地址 |
| `QINGJIN_DATA_DIR` | `-data` | `./data` | 数据目录（数据库、照片、密钥） |
| `QINGJIN_SECRET` | — | 自动生成 | 32 字节 base64 密钥，用于加密账号密码；不设则写入 `<data>/secret.key` |
| `QINGJIN_ADMIN_USER` | — | 空 | 初始管理员用户名；库中已有用户时忽略 |
| `QINGJIN_ADMIN_PASS` | — | 随机 | 初始管理员密码；不设则启动日志中打印随机密码 |

**公网部署务必设置 `QINGJIN_ADMIN_USER` / `QINGJIN_ADMIN_PASS`**，否则任何人访问 `/` 都能注册。

`<data>/secret.key` 一旦丢失，已保存的账号密码将无法解密（需重新录入）。备份数据目录时请一并备份。

## 使用流程

1. 注册并登录平台。
2. **学生账号 → 添加账号**：填写学校全称（输入时会自动匹配今日校园学校库）、学号、密码。
3. **登录方式二选一**
   - 点「账密登录」：后端直连学校 CAS/IAP 完成登录；有图形验证码时弹出输入框。可用 Go 原生流程，无需 Java 组件。
   - 或点「导入 Cookie」：在浏览器登录今日校园网页版，复制请求头 Cookie 粘贴进来。
4. **绑定设备 ID**：点「设备码」生成二维码，用**今日校园 App 的扫一扫**打开，页面会显示设备 ID 并自动回填。设备 ID 必须来自你平时打卡的那台手机。
5. **设置默认位置**：填写签到点附近的地址与经纬度（GCJ-02）。坐标需落在学校签到点半径内，否则会被判定为范围外。
6. **上传签到照片**（任务要求照片时必需）。
7. **任务执行 → 拉取今日任务**，选中任务点「执行」；或在「自动签到」标签页创建计划，由服务端按设定时段运行。
8. **执行记录**核对结果；「结果待核对」表示需要人工到学校系统确认。

## 扫码授权能拿到什么

扫码页运行在今日校园 App 的 WebView 内，通过官方 Cordova 桥可以取到：

| 字段 | 来源 | 用途 |
| --- | --- | --- |
| `deviceId` | `MAMPDevice.getDeviceInfo` / `Device.getDeviceInfo` | **签到提交时使用**，服务端据此校验设备授权 |
| `uuid` | 同上（系统级标识） | 仅展示，签到不使用 |
| 机型 / 系统版本 | 插件返回，缺失时从 App 的 UA 解析 | 写进请求头 UA，影响风控评分 |
| 构建号 | 同上 | UA 里的 `Build/xxx` 段 |

取不到时会依次降级：`deviceready` 事件 → `cordova.exec` 双插件轮询（1.5 秒一次，共 10 秒）→
`BH_MOBILE.device.getDeviceId` → `window.device.uuid` → 展示引导页。

> **扫码不会替你登录。** 有些同类工具会额外调用 App 的
> `BH_MOBILE_SDK.http.sendGetRequest` 请求 `setCookieByToken` 拿 `cookieToken` + `userId` + `pwid`，
> 用这个凭证直接换取学校会话，从而跳过账号密码和验证码。本项目**未实现**这条路径：
> 它依赖 App 内部未公开的桥接能力，稳定性差且涉及账号凭证外泄风险。

## 协议说明（二次开发参考）

签到提交的关键点（`internal/cpdaily`）：

```
bodyString  = base64( AES-128-CBC-PKCS7( json(表单), key=fkey_cat ) )
IV          = 01..08 09 01..07
fkey_cat    = hru89rwr87Nthifa        (interleave("h8r7uN8t" + catSecret))
Cpdaily-Extension = base64( DES-CBC( json(设备信息), "b3L26XNL" ) )
CpdailyInfo       = base64( DES-CBC( json(设备信息), "ST83=@XV" ) )
version     = first_v4, calVersion = firstv
```

- `fkey_cat` 由 `getSecretKey` 接口协商派生，实测全平台固定；服务端轮换后可用 `FetchSecretKey` 重新获取（代码内已内嵌 RSA 公私钥）。
- 若返回「今日校园版本过低」，说明 `bodyString` 解密失败，通常是密钥或客户端版本已轮换。

### Android 与 iOS 协议差异

两种客户端的提交格式不同，账号里可切换「设备平台」。差异来自实机抓包比对：

| 项目 | Android | iOS |
| --- | --- | --- |
| `deviceType` 头 | `1` | `2` |
| `systemName` | `android` | `iOS` |
| User-Agent | `(Linux; Android 14; MODEL Build/XXX; wv) … okhttp/4.12.0` | `(iPhone; CPU iPhone OS 18_2 like Mac OS X) … Mobile/15E148` |
| `Cpdaily-Extension` | ✓（DES `b3L26XNL`） | ✓（同密钥，额外含 `cpdailyVersion`） |
| `CpdailyInfo` | ✓（DES `ST83=@XV`） | 不发送 |
| `extension` / `sign` 头 | ✓ | 不发送 |
| `wisDeviceId` 头 | 无 | 有（设备指纹） |
| Cookie | 自动 | 手动拼，**必须含 `MOD_AUTH_CAS`** |
| `sign` 请求体字段 | 有 | 有（服务端不校验内容） |

> `bodyString` 的加密方式两者完全一致：AES-128-CBC + 固定 IV + `fkey_cat`。

### 登录（CLOUD / IAP 学校）

网页版统一认证的密码**不是明文**，而是：

```
password = "{rsa}" + base64( RSA_PKCS1v15(明文) )
```

`{rsa}` 前缀是服务端识别加密方式的标记，缺失会被按明文处理并返回 `FAIL_UPNOTMATCH`。
RSA-1024 公钥写在登录页的 `chunk-common.js` 里，各校可能不同——本项目运行时会
从登录页动态抓取（失败才回退内置公钥），因此不依赖硬编码。

登录流程：

1. `GET {idsUrl}/login?service=...` 建立 `CONVERSATION` 会话
2. `POST {idsUrl}/security/lt` 取 `_lt`（`code:200` 为成功，与业务接口的 `code:0` 不同）
3. `POST {idsUrl}/doLogin`，form-urlencoded，带 `X-Requested-With`、`deviceId`、`fingerprintId` 头
4. 成功返回 `{"resultCode":"REDIRECT"}` + `CASTGC`；连续失败会转 `CAPTCHA_NOTMATCH` + `needCaptcha`

> CAS（`NOTCLOUD`）学校走另一条链路：解析登录表单、`#pwdEncryptSalt` 做 AES 加密、可选图形验证码。
- 设备未授权（code=3）需要先用该设备在官方 App 正常签到一次，或找辅导员在「辅导猫 → 手机授权管理」授权。

接口端点：

| 类型 | 列表 | 详情 | 提交 |
| --- | --- | --- | --- |
| 签到 | `/wec-counselor-sign-apps/stu/sign/getStuSignInfosInOneDay` | `.../detailSignTaskInst` | `.../completeSignIn` |
| 查寝 | `/wec-counselor-attendance-apps/student/attendance/getStuAttendacesInOneDay` | `.../detailSignInstance` | `.../submitSign` |
| 收集 | `/wec-counselor-collector-apps/stu/collector/queryCollectorProcessingList` | `.../detailCollector` | `.../submitForm` |

## 字段说明

添加账号时的几项参数，含义和取值来源如下：

| 字段 | 含义 | 从哪来 |
| --- | --- | --- |
| 设备 ID | 今日校园 App 生成的设备唯一标识，提交签到时放在 `CpdailyInfo` 里，服务端据此校验「这台设备是否被账号授权」 | 用「设备码」扫码自动获取（取 `deviceId`，不是 `uuid`） |
| 设备型号 | 写进请求头 UA 和 `CpdailyInfo.model`，例如 `23127PN0CC`。做成与你真机一致可降低风控概率 | 手机「设置 → 关于手机 → 设备名称/型号」 |
| Android 版本 | 同上，写进 UA 的 `Android 14` 与 `systemVersion` | 手机「设置 → 关于手机 → Android 版本」，只填数字如 `14` |
| 异常原因 | 签到表单里的 `abnormalReason`。定位不在签到点范围内时会带这个说明提交，常规情况填「在校」 | 按学校要求填写，一般保持「在校」 |
| 经度 / 纬度 | 签到定位坐标（GCJ-02）。服务端会算你与签到点的距离，超出半径会判定为范围外 | 「在地图上选点」，拖动准星到你的实际位置 |

> 设备型号和系统版本**不会**参与设备授权校验（授权只看 `deviceId`），但填得不真实容易被风控标记。
> 建议直接照抄你打卡那台手机的实际值。

### 位置自动获取

不需要手动选点或填坐标。点「自动获取位置」，程序会：

1. 登录后拉取当日查寝任务
2. 读取任务详情里的 `signPlaceSelected`（学校设置的签到点）
3. 把中心点坐标和半径直接写入账号

这样提交时的位置就是学校认可的点，不会出现「不在签到范围内」。

> 如果任务没有设置签到点（自由签到型），会提示需要手动处理。

### 设备信息全部来自扫码

扫码时回传的字段会原样用于提交：

| 字段 | 用途 |
| --- | --- |
| `deviceId` | 设备校验（`CpdailyInfo` / `Cpdaily-Extension`） |
| `deviceModel` / `systemVersion` / `buildId` | 设备信息与 UA |
| **`userAgent`** | **原样作为请求的 User-Agent**，与扫码时 App 完全一致 |
| `wisDeviceId` | iOS 设备指纹头 |
| `platform` | 决定走 Android 还是 iOS 协议 |

优先使用扫码拿到的原始 UA，而不是拼接生成——这样发出的请求特征与你手机上的今日校园 App 完全一致。

## 开放 API

所有接口返回统一结构：

```json
{ "ok": true, "data": { } }
{ "ok": false, "error": { "code": "unauthorized", "message": "请先登录" } }
```

认证：请求头 `Authorization: Bearer <token>`（登录/注册接口返回）。完整清单见 [docs/API.md](docs/API.md)。

```bash
# 登录拿 token
curl -X POST http://localhost:8090/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"secret123"}'

# 拉取今日任务
curl -X POST http://localhost:8090/api/tasks/sync \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"accountId":1}'

# 执行任务
curl -X POST http://localhost:8090/api/tasks/run \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"accountId":1,"taskId":1}'
```

健康检查与自检（无需登录）：

```bash
curl http://localhost:8090/api/health
curl http://localhost:8090/api/selftest   # 校验 AES/DES/密钥派生与数据库
```

## 目录结构

```
cmd/qingjin/          入口：HTTP 服务 + 调度器 + 优雅退出
internal/cpdaily/     今日校园协议层（加密、学校解析、登录、任务、提交）
internal/store/       SQLite 数据访问
internal/service/     业务编排（认证、账号、任务执行、设备票据）
internal/scheduler/   自动计划调度
internal/server/      HTTP 路由与处理器
internal/secure/      密码哈希、字段加密、token
internal/config/      运行配置
web/                  内嵌前端（dist 为产物，直接编辑即可）
deploy/               Dockerfile、systemd 单元
docs/                 API 文档
```

## 前端开发

前端没有构建步骤，直接改 `web/dist/` 下的文件后重新 `go build` 即可（`go:embed` 会重新打包）。静态资源带内容指纹，编译后自动失效旧缓存。

## 已知限制


- **验证码**只支持图形验证码手动输入，不支持滑块/人脸/短信。
- **设备 ID**：今日校园返回 `deviceId`（App 生成，签到使用）和 `uuid`（系统标识）。扫码页两者都会展示，绑定的是 `deviceId`。
- **扫码只绑定设备指纹，不等于登录**：扫码拿的是设备标识，不是账号会话。绑定后仍需在电脑端输入学号密码完成学校登录。
- **信息收集**仅实现查询，表单填写需要按学校题目适配。
- 学校接口与加密规则可能随时变化；执行前请在学校系统确认任务要求。

## 免责声明

仅供个人学习与自用。虚拟定位、代签到可能违反校规与平台条款，使用本项目产生的任何后果由使用者自行承担。请勿用于付费代挂或大规模代签。

## 许可证

MIT
