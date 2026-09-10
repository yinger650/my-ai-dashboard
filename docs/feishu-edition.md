# Feishu edition（`board-server-feishu`）

与 **Personal**（`board-server`，密码 + 单库）同仓库、同一套看板/ingest/Store。飞书版是另一条二进制、另一套部署，用来让本企业飞书用户各自拥有一份看板。

主版本改 handlers、卡片、Event 协议时，飞书版自动带上。不要为飞书长期开分支。

## 人怎么进

飞书网页应用主页配置为站点**根路径** `/`。用户飞书登录后看到的是自己的看板，URL 里没有 slug。

## Agent 怎么报

每个用户有一个 8 位 `[a-z0-9]` slug。上报地址是：

```
https://<新域名>/<slug>/ingest/v1/events
```

`AGENTBOARD_URL` 设成 `https://<新域名>/<slug>`（`report.py` 会再拼 `/ingest/v1/events`）。鉴权仍是 Machine Token `abp_m_…`。slug 只做路由，不当密钥。

根路径 `/ingest/v1` 在飞书 edition **不存在**。浏览器打开 `/{slug}` 会回到 `/`。

## 部署

完整步骤（构建、systemd、nginx/TLS、飞书后台发版、验收）见 [飞书版部署手册](./feishu-deploy.md)。摘要：

1. 飞书开放平台创建企业自建应用，网页应用主页 = `https://<新域名>/`，重定向 URL = `https://<新域名>/auth/feishu/callback`。打开方式建议独立窗口。改主页后必须创建版本并发布。
2. 同一 git tag 构建 `board-server-feishu`。
3. systemd 用 [deploy/board-server-feishu.service](../deploy/board-server-feishu.service)，环境见 [deploy/board-server-feishu.env.example](../deploy/board-server-feishu.env.example)。
4. nginx 与 Personal 相同，只换 `server_name` 和上游端口。现网模板：[deploy/nginx-board.min-wang.com.conf](../deploy/nginx-board.min-wang.com.conf)。

数据目录：

- `{ABP_DATA_DIR}/control.db`：飞书用户、slug、session
- `{ABP_DATA_DIR}/workspaces/{slug}/board.db`：该用户的 AgentBoard Personal 库
- `{ABP_DATA_DIR}/workspaces/{slug}/artifacts/`

Personal 现网 `board.yinger650.com` 继续跑 `board-server`，不要把飞书版配到同一 `ABP_DATA_DIR`。
