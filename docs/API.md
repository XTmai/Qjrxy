# 青衿开放 API

所有接口前缀 `/api`，返回统一信封：

```json
{ "ok": true, "data": {} }
```

```json
{ "ok": false, "error": { "code": "bad_request", "message": "请填写学校全称" } }
```

错误码：`bad_request`(400)、`unauthorized`(401)、`not_found`(404)、`quota_exhausted`(402)、`failed`(400)。

认证：登录或注册返回 `token`，之后请求头带 `Authorization: Bearer <token>`。
（也支持 `qj_token` cookie，浏览器端使用。）

---

## 公开接口

### `GET /api/health`

健康检查。

```json
{ "ok": true, "data": { "status": "ok", "time": "2026-11-01T18:00:00+08:00", "name": "青衿 · 今日校园签到与查寝" } }
```

### `GET /api/selftest`

校验 AES-128-CBC、DES、密钥派生与数据库连通性。失败时返回 500。

```json
{ "ok": true, "data": { "crypto": "ok", "database": "ok", "appVersion": "9.9.24" } }
```

### `POST /api/auth/register`

```json
{ "username": "demo", "password": "secret123", "nickname": "演示" }
```

返回 `{ "user": {...}, "token": "..." }`。用户名 ≥ 3 位，密码 ≥ 6 位。

### `POST /api/auth/login`

```json
{ "username": "demo", "password": "secret123" }
```

### `POST /api/auth/logout`

注销当前 token。

### `GET /api/auth/me`

返回 `{ "user": {...} }`。

### `POST /api/auth/password` 🔒

```json
{ "oldPassword": "old", "newPassword": "newpass123" }
```

---

## 学校

### `GET /api/schools?q=<关键词>` 🔒

搜索今日校园学校库，最多返回 80 条。

```json
{ "ok": true, "data": { "schools": [ { "id": "030bc09c-...", "name": "广西科技大学" } ], "total": 12 } }
```

---

## 账号

账号对象字段（`account`）：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | number | 账号 ID |
| `alias` | string | 显示名 |
| `schoolName` | string | 学校全称 |
| `username` | string | 学号 |
| `tenantId` | string | 学校租户 ID |
| `host` | string | 校园门户域名 |
| `joinType` | string | `CLOUD`(IAP) 或 `CAS` |
| `loginStatus` | string | `authorized` / `expired` / `unknown` |
| `deviceId` | string | 设备 ID |
| `deviceModel` / `systemVersion` / `buildId` | string | 设备指纹 |
| `address` / `lon` / `lat` | string / number | 默认签到位置 |
| `abnormalReason` | string | 异常原因，默认「在校」 |
| `hasCookie` | bool | 是否已登录 |
| `photoUrl` | string | 照片地址，无照片为空 |

### `GET /api/accounts` 🔒

```json
{ "ok": true, "data": { "accounts": [ ... ] } }
```

### `POST /api/accounts` 🔒

```json
{
  "schoolName": "广西科技大学",
  "username": "20230001",
  "password": "校园网密码",
  "deviceId": "可选",
  "deviceModel": "23127PN0CC",
  "systemVersion": "14",
  "address": "南苑 3 号楼",
  "lon": 109.428,
  "lat": 24.326,
  "abnormalReason": "在校",
  "cookies": "可选，MOD_AUTH_CAS=xxx; sessionToken=yyy"
}
```

服务端会解析学校入口并落库。`schoolName` 与 `username` 必填，`password` 与 `cookies` 至少提供一个。

### `GET /api/accounts/{id}` 🔒

返回 `{ "account": {...}, "tasks": [...] }`。

### `PATCH /api/accounts/{id}` 🔒

字段同创建；`password` 留空表示不修改。

### `DELETE /api/accounts/{id}` 🔒

级联删除该账号的计划与执行记录。

### `POST /api/accounts/{id}/login` 🔒

用保存的密码发起登录。无需验证码时：

```json
{ "ok": true, "data": { "done": true, "message": "登录成功" } }
```

需要图形验证码时：

```json
{
  "ok": true,
  "data": {
    "done": false,
    "ticket": "xxxxx",
    "needCaptcha": true,
    "captcha": "data:image/png;base64,...",
    "expiresAt": "2026-11-01T18:15:00+08:00"
  }
}
```

### `POST /api/accounts/{id}/captcha` 🔒

```json
{ "ticket": "xxxxx", "code": "a1b2" }
```

### `POST /api/accounts/{id}/cookies` 🔒

```json
{ "cookies": "MOD_AUTH_CAS=xxx; sessionToken=yyy" }
```

会立即用该 Cookie 拉一次任务验证有效性。

### `POST /api/accounts/{id}/verify` 🔒

返回 `{ "loginStatus": "authorized" | "expired" | "unknown", "message": "..." }`。

### `POST /api/accounts/{id}/photo` 🔒

`multipart/form-data`，字段名 `photo`，支持 jpg/png/webp，≤ 5MB。

### `GET /api/accounts/{id}/photo` 🔒

返回图片二进制。

### `GET /api/accounts/{id}/photos` 🔒

返回该账号照片池：`{ "photos": [{ "id": 1, "url": "/api/accounts/1/photos/1", "createdAt": "..." }] }`。

### `POST /api/accounts/{id}/photos` 🔒

`multipart/form-data`，字段名 `photos` 可重复；单张 jpg/png/webp ≤ 5MB，单次最多 10 张，每账号最多 10 张。逐文件返回成功列表与 `failed`（文件名 + 原因）。

### `GET /api/accounts/{id}/photos/{photoId}` 🔒

返回池内照片二进制，支持与单张照片相同的 `?w=` 缩略图参数。

### `DELETE /api/accounts/{id}/photos/{photoId}` 🔒

删除池内照片及其文件。

### `POST /api/accounts/{id}/device-ticket` 🔒

生成设备扫码票据与二维码：

```json
{
  "ok": true,
  "data": {
    "ticket": "...",
    "url": "https://your-host/device.html?t=...",
    "expiresAt": "2026-11-01T18:15:00+08:00",
    "qrcode": "data:image/png;base64,..."
  }
}
```

二维码 15 分钟有效、仅可使用一次。

### `POST /api/device/submit`

设备页调用，**无需登录**（用票据授权）。提交后**不消费票据**，主页面靠轮询取结果：

```json
{
  "ticket": "...",
  "deviceId": "aabbccdd-...",
  "uuid": "可选",
  "deviceModel": "23127PN0CC",
  "systemVersion": "14",
  "buildId": "可选",
  "systemName": "android | iOS",
  "appVersion": "可选",
  "wisDeviceId": "可选，iOS 指纹"
}
```

### `GET /api/device/poll/{ticket}` 🔒

电脑端轮询扫码结果。拿到结果时**自动落库并消费票据**：

```json
{ "ok": true, "data": { "pending": true } }
```

```json
{
  "ok": true,
  "data": {
    "pending": false,
    "collected": true,
    "deviceId": "aabbccdd-...",
    "deviceModel": "V2405A",
    "systemVersion": "16",
    "buildId": "AP2A.240705.005",
    "platform": "android",
    "uuid": "11111111-..."
  }
}
```

完整流程：

```
POST /api/accounts/{id}/device-ticket   → 拿 ticket + 二维码
            ↓（用户用今日校园 App 扫码）
POST /api/device/submit                 → 扫码页写入设备信息
            ↓（前端每 1.5 秒）
GET  /api/device/poll/{ticket}          → pending=false 时自动回填并消费
```

---

## 任务

### `GET /api/tasks` 🔒

返回全部账号的缓存待办。

### `POST /api/tasks/sync` 🔒

```json
{ "accountId": 1 }
```

从学校系统拉取签到、查寝、信息收集三类待办，覆盖该账号缓存。

### `POST /api/tasks/run` 🔒

```json
{ "accountId": 1, "taskId": 1, "kind": "attendance" }
```

`taskId` 省略时自动挑第一个未处理任务；`kind` 可限定 `sign` / `attendance`。
每次调用扣 1 次额度，额度不足返回 402。

返回：

```json
{
  "ok": true,
  "data": {
    "logId": 12,
    "status": "success",
    "message": "已完成",
    "hint": "",
    "taskName": "晚间查寝"
  }
}
```

`status`：`success` / `failed` / `pending`（结果待核对，如需要人脸或定位异常）。
失败时 `hint` 给出可操作建议（设备未授权、坐标超范围、版本过低等）。

---

## 计划

计划对象：

| 字段 | 说明 |
| --- | --- |
| `accountId` | 绑定的账号 |
| `name` | 计划名称 |
| `kind` | `attendance` / `sign` |
| `mode` | `school`（跟随学校下发时间） / `custom`（自定义时段） |
| `windowStart` / `windowEnd` | `HH:MM`，`mode=custom` 时生效，支持跨天 |
| `days` | 执行星期，`0` = 周一 … `6` = 周日 |
| `enabled` | 是否启用 |
| `lastRunAt` / `lastResult` | 最近执行时间与结果 |

### `GET /api/plans` 🔒

返回 `{ "plans": [...] }`，附带 `accountAlias` / `schoolName` / `accountUsername`。

### `POST /api/plans` 🔒

```json
{
  "accountId": 1,
  "name": "晚间查寝计划",
  "kind": "attendance",
  "mode": "custom",
  "windowStart": "20:00",
  "windowEnd": "23:30",
  "days": [0, 1, 2, 3, 4],
  "enabled": true
}
```

### `PATCH /api/plans/{id}` 🔒

字段同上。

### `POST /api/plans/{id}/toggle` 🔒

```json
{ "enabled": false }
```

### `DELETE /api/plans/{id}` 🔒

---

## 记录

### `GET /api/logs` 🔒

查询参数：`scope=today|week|all`、`status=success|failed|pending|executing|all`、`kind=attendance|sign|collect|all`、`limit`（默认 20）、`offset`。

```json
{
  "ok": true,
  "data": {
    "logs": [
      {
        "id": 12,
        "accountId": 1,
        "accountAlias": "演示账号A",
        "schoolName": "广西科技大学",
        "taskName": "晚间查寝",
        "kind": "attendance",
        "source": "auto",
        "status": "success",
        "message": "已完成",
        "detail": "{\"code\":\"0\",\"inRange\":true,...}",
        "createdAt": "2026-11-01T20:05:00+08:00"
      }
    ],
    "total": 1
  }
}
```

`detail` 是 JSON 字符串，包含坐标、签到点距离、服务端返回码等。

### `GET /api/logs/{id}` 🔒

返回 `{ "log": {...} }`。

---

## 工作台

### `GET /api/dashboard` 🔒

```json
{
  "ok": true,
  "data": {
    "stats": { "todayTotal": 4, "todaySuccess": 2, "todayFailed": 1, "todayPending": 1 },
    "accounts": [ ... ],
    "tasks": [ ... ],
    "pendingTask": 2,
    "plans": 2,
    "activePlans": 1,
    "recentLogs": [ ... ],
    "user": { ... },
    "today": "11月1日周日"
  }
}
```

---

## 额度

### `POST /api/quota/redeem` 🔒

```json
{ "code": "QJ-A1B2-C3D4" }
```

当前实现：卡密为**一次性**，且必须先在 `redeem_codes` 表预置（`code`, `amount`）。兑换时用条件 UPDATE 原子占位 `used_by`，未入库、格式不符或已使用的码一律返回 400。

```bash
# 预置一张 30 次卡密
sqlite3 data/qingjin.db \
  "INSERT INTO redeem_codes (code, amount, created_at) VALUES ('QJ-A1B2-C3D4', 30, '2026-11-04T12:00:00+08:00');"
```

返回 `{ "user": {...}, "granted": 30, "message": "兑换成功" }`。

---

## 系统

### `GET /api/system/info` 🔒

```json
{
  "ok": true,
  "data": {
    "appVersion": "9.9.24",
    "apiVersion": "v1",
    "serverTime": "2026-11-01T18:00:00+08:00",
    "tokenTTLDay": 30,
    "features": ["manual-sign", "auto-plan", "multi-account", "photo", "cookie-import", "password-login", "device-id"]
  }
}
```
