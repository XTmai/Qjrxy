# 部署步骤

## 1. 上传二进制

```bash
# 本机交叉编译（已在 bin/ 生成）
scp bin/qjrxy-linux-amd64 user@server:/tmp/qjrxy

# 服务器上安装
sudo install -m 755 /tmp/qjrxy /usr/local/bin/qjrxy
sudo useradd -r -s /sbin/nologin qjrxy 2>/dev/null || true
sudo mkdir -p /var/lib/qjrxy && sudo chown -R qjrxy:qjrxy /var/lib/qjrxy
```

## 端口配置

默认使用 **8090**。若仍被占用，有三种改法（优先级从高到低）：

```bash
# 1) 命令行参数（最高优先级）
./qjrxy -addr :9000 -data /var/lib/qjrxy/data

# 2) 环境变量
QINGJIN_ADDR=:9000 ./qjrxy -data /var/lib/qjrxy/data

# 3) systemd 单元里同时改 -addr 与 QINGJIN_ADDR 两处
```

查看端口占用：

```bash
sudo ss -lntp | grep -E ':(8080|8090|9000)'
```

只用 Nginx 对外提供 80/443 时，可把监听收紧到本机，避免直接暴露：

```bash
QINGJIN_ADDR=127.0.0.1:8090 ./qjrxy -data /var/lib/qjrxy/data
```

## 2. 首次初始化（重要）

注册口默认只在**库中还没有任何用户**时开放：第一次访问页面注册的那个账号就是实例主人，
之后注册自动关闭（返回 403 `register_closed`）。需要多人自助注册时再显式开启：

```bash
QINGJIN_REGISTER_OPEN=true ./qjrxy -data /var/lib/qjrxy/data
```

如果希望跳过页面注册、直接由环境变量建号：

```bash
export QINGJIN_ADMIN_USER=admin
export QINGJIN_ADMIN_PASS='换成你的强密码'
```

首次启动时若库中无用户，会自动创建这个账号；库中已有用户时该变量被忽略。

> 登录接口有频率限制（同一 IP + 账号名 5 次/分钟，注册 3 次/小时），
> 超限返回 429，属正常防护，不是故障。

## 3. 运行

```bash
sudo -u qjrxy QINGJIN_ADMIN_USER=admin QINGJIN_ADMIN_PASS='强密码' \
  /usr/local/bin/qjrxy -addr :8090 -data /var/lib/qjrxy/data
```

或使用 systemd（推荐）：

```bash
sudo cp deploy/qjrxy.service /etc/systemd/system/
sudo vim /etc/systemd/system/qjrxy.service   # 改掉 QINGJIN_ADMIN_PASS
sudo systemctl daemon-reload
sudo systemctl enable --now qjrxy
sudo journalctl -u qjrxy -f
```

或 Docker：

```bash
docker build -f deploy/Dockerfile -t qjrxy .
docker run -d --name qjrxy -p 8090:8090 -v $PWD/data:/data \
  -e QINGJIN_ADMIN_USER=admin -e QINGJIN_ADMIN_PASS='强密码' qjrxy
```

## 4. 验证

```bash
curl http://127.0.0.1:8090/api/health
curl http://127.0.0.1:8090/api/selftest      # crypto / database 都应为 ok
```

浏览器打开 `http://服务器IP:8090`，用管理员账号登录。

## 5. 反向代理（建议）

扫码功能需要**手机能访问服务器地址**，所以要用公网域名或 IP。

Nginx 配置：

```nginx
server {
    listen 80;
    server_name your.domain.com;

    location / {
        proxy_pass http://127.0.0.1:8090;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
        proxy_read_timeout 180s;   # 登录/签到可能较慢
    }
}
```

`X-Forwarded-Proto` / `X-Forwarded-Host` 必须传，否则设备二维码里的地址会是内网 IP。

## 6. 数据备份

整个 `/var/lib/qjrxy/data` 目录：

- `qjrxy.db` —— 数据库
- `secret.key` —— 账号密码加密密钥（**丢失后已保存的密码无法解密**）
- `photos/` —— 签到照片

```bash
sudo tar czf qjrxy-backup-$(date +%F).tar.gz -C /var/lib/qjrxy data
```

## 7. 常见问题

| 现象 | 原因 |
| --- | --- |
| 设备二维码扫出来是内网地址 | Nginx 没传 `X-Forwarded-Host` |
| 登录超时 | 学校认证服务器慢，加大 `proxy_read_timeout` |
| 签到返回「更换手机频繁」 | 设备 ID 与该账号已绑定设备不一致，需用真机的 deviceId |
| `secret.key` 丢失 | 已保存的账号密码需重新录入 |
