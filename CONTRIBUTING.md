# 版本管理约定

## 分支结构

| 分支 | 用途 |
| --- | --- |
| `main` | 稳定版本，可部署 |
| `ui-wheat` | 小麦校园 UI 实验分支（麦金暖色主题） |
| `ui-revert` | 旧版蓝色青衿 UI（当前使用） |
| `feature/*` | 新功能开发 |

## 提交规范

```
<type>: <简短描述>

<详细说明（可选）>
```

type 取值：
- `feat` 新功能
- `fix` 修 bug
- `refactor` 重构
- `style` 样式调整
- `docs` 文档
- `chore` 杂项

## 常用操作

```bash
# 查看改动
git status
git diff

# 提交
git add -A
git commit -m "fix: 修复删除按钮无响应"

# 切换 UI 版本
git checkout ui-wheat     # 小麦校园风格
git checkout ui-revert    # 蓝色青衿风格

# 回到稳定版
git checkout main

# 查看历史
git log --oneline --graph --all
```

## 发布前检查

```bash
# 后端
go vet ./...
go test ./...

# 前端
node scripts/check-frontend.mjs

# 编译
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o bin/qingjin-linux-amd64 ./cmd/qingjin
```

## 注意

- `data/` 目录不进版本控制（含数据库、密钥、照片）
- `secret.key` 丢失后已保存的账号密码无法解密，务必备份
- 修改前端后要重新 `go build`，因为资源是 `go:embed` 打进去的
