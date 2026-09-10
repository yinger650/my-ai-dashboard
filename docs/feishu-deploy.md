# 飞书版 AgentBoard 部署手册

把 **`board-server-feishu`** 部署成企业内网页应用：员工从飞书打开站点根路径 `/`，用飞书账号登录后各自看到自己的看板。Personal 版（`board-server`，密码 + 单库）继续独立运行，不要共用数据目录。

产品行为见 [feishu-edition.md](./feishu-edition.md)。下文按一次真实上线（`https://board.min-wang.com`）整理，可换域名复用。

---

## 0. 这次上线的结果

| 项 | 值 |
|---|---|
| 公网入口 | `https://board.min-wang.com` |
| 上游 | `board-server-feishu` 监听 `127.0.0.1:8091` |
| 二进制 | `/opt/agentboard/bin/board-server-feishu` |
| 环境文件 | `/etc/agentboard/board-server-feishu.env`（权限 `600`，不入库） |
| 数据目录 | `/var/lib/agentboard-feishu` |
| nginx | `/etc/nginx/conf.d/board.min-wang.com.conf`（仓内模板见 [deploy/nginx-board.min-wang.com.conf](../deploy/nginx-board.min-wang.com.conf)） |
| 证书 | Let's Encrypt，webroot `/var/www/acme` |
| 飞书回调 | `https://board.min-wang.com/auth/feishu/callback` |
| Agent 上报 | `https://board.min-wang.com/<slug>/ingest/v1/events`（slug 在看板设置页） |

Personal 现网 `board.yinger650.com` 与本机 `board-client` **不要动**。

---

## 1. 前置条件

1. **DNS**：`board.min-wang.com` 的 A 记录指向这台机的公网 IP。本机是 `8.153.69.148`。
2. **防火墙 / 安全组**：放行 `80`、`443`。上游只绑 `127.0.0.1`，不要把 `8091` 暴露到公网。
3. **本机已有 nginx**（Alibaba Cloud Linux 上常见 1.20）。证书与反代都走它。
4. **工具链**：Go **1.26+**、Node **20+**、pnpm。本机安装在 `/usr/local/go`、`/usr/local/node`，PATH 由 `/etc/profile.d/abp-toolchain.sh` 注入。
5. **飞书企业自建应用**：已有 App ID / App Secret。Secret 只写环境文件，不要提交 git，也不要贴进文档。
6. **数据隔离**：飞书版必须用独立 `ABP_DATA_DIR`。禁止指向 Personal 的 `/var/lib/agentboard`。

国内拉 Go 模块可设：

```bash
export GOPROXY=https://goproxy.cn,direct
```

下载很慢时再用仓库约定的 HTTP 代理。

---

## 2. 构建

仓库根目录有一个 `pnpm-workspace.yaml`，但不是完整 workspace。根上直接 `make build` 可能在 `pnpm install` 阶段失败。按下面拆开做。

```bash
cd /workspace/my-ai-dashboard

# 前端（嵌入进 Go 二进制）
cd web
pnpm install --frozen-lockfile --ignore-workspace
pnpm build
cd ..

# 只编飞书版
mkdir -p bin
go build -ldflags "-X main.version=$(git rev-parse --short HEAD) -X main.commit=$(git rev-parse HEAD) -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o bin/board-server-feishu ./cmd/board-server-feishu
```

`web/dist/` 构建产物不要提交。保留 `web/dist/.gitkeep`。

---

## 3. 安装二进制与 systemd

```bash
install -d -m 755 /opt/agentboard/bin /etc/agentboard /var/lib/agentboard-feishu
install -m 755 bin/board-server-feishu /opt/agentboard/bin/board-server-feishu
install -m 644 deploy/board-server-feishu.service /etc/systemd/system/board-server-feishu.service
```

从 [deploy/board-server-feishu.env.example](../deploy/board-server-feishu.env.example) 复制环境文件，填本机值：

```bash
install -m 600 deploy/board-server-feishu.env.example /etc/agentboard/board-server-feishu.env
```

本机实际关键项：

```bash
ABP_LISTEN_ADDR=127.0.0.1:8091
ABP_PUBLIC_URL=https://board.min-wang.com
ABP_DATA_DIR=/var/lib/agentboard-feishu
ABP_SECURE_COOKIES=true
ABP_TRUSTED_PROXY_CIDRS=127.0.0.1/32,::1/128
ABP_FEISHU_APP_ID=cli_xxxxxxxx
ABP_FEISHU_APP_SECRET=从飞书后台复制，不要入库
ABP_FEISHU_REDIRECT_URI=https://board.min-wang.com/auth/feishu/callback
ABP_CLIENT_UPDATE_SYNC=false
```

仓库 `.env` 里若已有 `FEISHU_APP_ID` / `FEISHU_APP_SECRET`，对应写到上面的 `ABP_FEISHU_*`。不要把 `.env` 整份拷进 systemd 环境文件。

```bash
systemctl daemon-reload
systemctl enable --now board-server-feishu
systemctl status board-server-feishu --no-pager
curl -sS http://127.0.0.1:8091/health/live
```

`/health/live` 应返回健康。日志：`journalctl -u board-server-feishu -f`。

---

## 4. nginx 反代与 HTTPS

### 4.1 先上 HTTP，给 ACME 留 webroot

1. 建目录：`mkdir -p /var/www/acme`
2. 把仓内 [deploy/nginx-board.min-wang.com.conf](../deploy/nginx-board.min-wang.com.conf) 拷到 `/etc/nginx/conf.d/board.min-wang.com.conf`。
3. **第一次还没有证书时**，先只启用 `listen 80` 那一段（含 `/.well-known/acme-challenge/`），`location /` 可以暂时 `proxy_pass http://127.0.0.1:8091;`，不要 `return 301 https://...`。
4. `nginx -t && systemctl reload nginx`

Alibaba Cloud nginx **1.20** 不要写 `http2 on;`，应写：

```nginx
listen 443 ssl http2;
listen [::]:443 ssl http2;
```

需要 `map $http_upgrade $connection_upgrade`（本机 `/etc/nginx/nginx.conf` 里 Personal 站点已经有这条）。

### 4.2 申证书

```bash
certbot certonly --webroot -w /var/www/acme -d board.min-wang.com
```

证书路径：

- `/etc/letsencrypt/live/board.min-wang.com/fullchain.pem`
- `/etc/letsencrypt/live/board.min-wang.com/privkey.pem`

续期后重载 nginx（本机 hook：`/etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh`）。

### 4.3 再开 443，80 跳 HTTPS

证书齐了之后启用模板里的 443 server，80 的 `/` 改为 `return 301 https://$host$request_uri;`。`nginx -t && systemctl reload nginx`。

外网自检：

```bash
curl -sSI https://board.min-wang.com/health/live
```

应看到 `200`，且走 TLS。

---

## 5. 飞书开放平台（网页应用）

OAuth **重定向 URL 不能靠 App Secret 调 Open API 写入**，必须在开发者后台手工填，并发布一个版本。缺这一步时，换 token 会返回飞书错误 **20029**（redirect_uri 未配置或不匹配）。

打开 [飞书开放平台](https://open.feishu.cn/app) → 选中企业自建应用。

### 5.1 加「网页应用」能力

应用默认能力经常是 **机器人**。飞书版看板走的是 **网页应用**：

1. **添加应用能力** → 勾选 **网页应用**（机器人可保留，看板不依赖它）。
2. **网页应用** → 桌面端 / 移动端主页填：

   `https://board.min-wang.com/`

3. **打开方式**建议：**在浏览器中打开新页面**（独立窗口）。飞书内嵌 webview 对 Cookie / 跳转更挑剔。

主页必须是站点**根路径** `/`。不要填 `/{slug}`。

### 5.2 安全设置

**安全设置**里填：

| 项 | 值 |
|---|---|
| 重定向 URL | `https://board.min-wang.com/auth/feishu/callback` |
| H5 可信域名 | `https://board.min-wang.com` |

必须 **https**，且与 `ABP_PUBLIC_URL` / `ABP_FEISHU_REDIRECT_URI` **逐字一致**（含有无尾斜杠：回调路径不要多余 `/`）。

重定向 URL 保存后往往立刻生效；**网页应用主页**要等版本发布才进员工端。

### 5.3 权限与可用范围

登录只用 OAuth `user_info`（open_id、姓名、头像）。一般不需要再开通讯录批量读权限。

**可用范围**至少包含要试用的人（本机第一次是应用所有者）。全员开放前先自己登录通。

### 5.4 创建版本并发布

1. **版本管理与发布** → **创建版本**（例如 `1.0.2`）。
2. 确认能力列表里有 **网页应用**，主页 URL 正确。
3. **保存** → **申请发布** / **发布**。企业自建应用按租户策略可能免审或要管理员审。
4. 发布成功后，从飞书工作台点开应用，应跳到 `https://board.min-wang.com/`，再进 `/auth/feishu/login` → 飞书授权 → `/auth/feishu/callback` → 自己的空看板。

改主页、打开方式、能力列表：再打一个小版本发布。只改重定向 URL 通常不用发版，但改完仍建议登录走一遍。

---

## 6. 登录与上报验收

1. 浏览器打开 `https://board.min-wang.com/`，应跳转飞书授权；回来后看到空看板（还没有机器）。
2. **设置**页复制 8 位 slug。Agent 上报地址是：

   ```
   https://board.min-wang.com/<slug>/ingest/v1/events
   ```

   项目 `.env` 里：

   ```bash
   AGENTBOARD_URL=https://board.min-wang.com/<slug>
   AGENTBOARD_TOKEN=abp_m_你在设置页创建的 token
   AGENTBOARD_PROVIDER=cursor
   ```

   `report.py` 会再拼 `/ingest/v1/events`。根路径 `/ingest/v1` 在飞书版 **不存在**。
3. 编码 Agent 上报步骤见 [agent-report-tutorial.md](./agent-report-tutorial.md)。

---

## 7. 升级

```bash
# 在仓库目录按第 2 节重新构建
install -m 755 bin/board-server-feishu /opt/agentboard/bin/board-server-feishu
systemctl restart board-server-feishu
curl -sS http://127.0.0.1:8091/health/live
```

环境文件与数据目录保持不动。飞书后台无需因二进制升级而发版，除非改了回调路径或主页 URL。

---

## 8. 常见问题

| 现象 | 处理 |
|---|---|
| `make build` / `pnpm install` 报 workspace | `cd web && pnpm install --frozen-lockfile --ignore-workspace` |
| nginx `http2 on` 报未知指令 | 1.20 写成 `listen 443 ssl http2` |
| certbot 失败 | 先确认 DNS 已指到本机、80 放行、`/.well-known/acme-challenge/` 指到 `/var/www/acme` |
| 飞书换 token `20029` | 后台「重定向 URL」未配或与 `ABP_FEISHU_REDIRECT_URI` 不一致 |
| 工作台点开还是旧主页 | 网页应用主页改完后必须 **创建版本并发布** |
| 登录后立刻掉线 | 确认 `ABP_SECURE_COOKIES=true`、反代传了 `X-Forwarded-Proto https`、站点是 https |
| 上报 404 | URL 漏了 8 位 slug，或误用了根路径 `/ingest/v1` |
| 和 Personal 数据搅在一起 | 立刻停飞书版，检查 `ABP_DATA_DIR` 是否误写成 Personal 目录 |

不要把 `ABP_FEISHU_APP_SECRET`、`.env`、`/etc/agentboard/*.env` 提交进 git。
