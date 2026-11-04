# Qjrxy

今日校园（Campusphere）查寝自动助手 —— 自部署单二进制工具。

绑定账号后，服务端会在查寝时间窗口内自动提交，会话过期自动恢复，全程无需打开 App。

> 本项目**只处理查寝任务**（宿舍考勤），二维码签到类任务不会执行。

![首页](docs/screenshots/home.png)

## 功能

- **自动查寝** — 按计划的星期与时段执行；任务未到时间自动等待，不产生无效提交
- **窗口驱动调度** — 同步一次任务后在本地计算唤醒时刻，不做高频轮询（每账号每天约 2 次请求）
- **会话自愈** — 登录态定期探测，失效时用保存的密码自动重登并同步任务
- **多账号** — 每个账号独立设备指纹，扫码获取真实设备信息，提交特征与 App 一致
- **自动登录** — 账密 AES-256-GCM 加密存储，图形验证码在界面内输入
- **自动定位** — 从学校下发的任务读取签到点与半径，无需手动选点
- **执行记录** — 成功 / 失败 / 待核对分级，7 日趋势，签到点距离可回溯
- **深色模式** · 移动端适配 · 单二进制部署

## 界面

| 账号管理 | 执行详情 |
| --- | --- |
| ![账号](docs/screenshots/accounts.png) | ![详情](docs/screenshots/detail.png) |

![设置](docs/screenshots/settings.png)

## 快速开始

### 方式一：下载二进制

从 [Releases](../../releases) 下载对应平台产物，直接运行：

```bash
./qjrxy -addr :8090 -data ./data
```

### 方式二：Docker

```bash
docker build -f deploy/Dockerfile -t qjrxy .
docker run -d -p 8090:8090 -v ./data:/app/data qjrxy
```

### 方式三：源码构建

需要 Go ≥ 1.26、Node ≥ 20：

```bash
make build          # 前端构建 + Go 编译 → bin/qjrxy
./bin/qjrxy -addr :8090 -data ./data
```

> 仓库已提交构建好的 `web/dist`，只改后端时直接 `go build` 即可，无需 node。

## 使用流程

1. 首次访问注册管理员账号（数据全部在你自己的服务器上）
2. **账号页 → 添加账号**：学校（自动匹配）+ 学号 + 密码 → 保存即登录，自动获取签到点
3. 点账号卡片 → **设备 → 扫码绑定设备**（用平时打卡的手机扫，App 右上角「＋」→ 扫一扫）
4. 点账号卡片 → **设备 → 上传照片**（查寝要求照片时必填）
5. 点账号卡片 → **计划 → 新建计划**：选星期与执行方式（跟随学校时间 / 自定义时段）
6. 完成。之后每天到点自动查寝，首页可看状态

## 配置

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `QINGJIN_ADDR` | `:8090` | 监听地址 |
| `QINGJIN_DATA_DIR` | `./data` | 数据目录（SQLite、照片、密钥） |
| `QINGJIN_SECRET` | 自动生成 | 32 字节密码加密密钥（base64） |
| `QINGJIN_ADMIN_USER` | 空 | 首次启动创建的管理员账号 |
| `QINGJIN_ADMIN_PASS` | 空 | 管理员密码 |
| `QINGJIN_QUOTA_ENABLED` | `false` | 额度/卡密体系开关（托管多用户运营场景才需要） |

## 架构

```
Go 后端（单二进制，go:embed 内嵌前端）
├── internal/cpdaily    今日校园协议层（登录 / 任务 / 提交 / 照片上传）
├── internal/scheduler  双循环调度器（计划执行 30s + 登录态探测按需）
├── internal/service    业务层（会话自愈、任务窗口计算、设备绑定）
├── internal/server     HTTP API
└── internal/store      SQLite（modernc.org/sqlite，无 CGO）
前端
└── web/frontend        React 19 + TypeScript + Tailwind v4 + shadcn/ui（Vite 构建）
```

## 开发

```bash
make frontend-dev    # 前端热更新（/api 代理到 :8090）
make build           # 完整构建
make test            # Go 测试
make linux           # 交叉编译 Linux amd64/arm64
```

## 免责声明

本项目仅供个人学习与研究。请使用**本人或已获授权的账号**，遵守学校规定与相关法律法规。
使用本工具产生的一切后果由使用者自行承担。

## 致谢

- 界面基于 [OpeniLink Hub](https://github.com/openilink/openilink-hub)（MIT）的 shadcn/ui 骨架改造
- 协议细节参考：[qingjin-cpdaily](https://github.com/3238614968/qingjin-cpdaily)、
  [CpdailyHelper](https://github.com/YVFENGXS/CpdailyHelper)、
  [jrxyzuixingnx](https://github.com/maserpoassr/jrxyzuixingnx)、
  [campus-device-id](https://github.com/2257929377/campus-device-id)

## License

[MIT](LICENSE)
