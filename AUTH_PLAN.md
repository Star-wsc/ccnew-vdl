# DouBi 鉴权与外网暴�?�?决策计划（已拍板�?
> 状态：**决策已定，未改代�?*  
> 拍板时间�?026-09-13  
> 原则：先按本计划实现 Phase 1�?；核心解�?合并逻辑不碰�?*你验收前�?push、不发版**

---

## 0. 已确认决策（锁定�?
| # | 决策�?| 结论 |
|---|---|---|
| D1 | 鉴权形�?| **用户�?+ 密码登录**（非�?Token�?|
| D2 | 主入�?| 用户�?**Lucky / Hermes 等反代域�?*访问；HTTPS 在反代终结；应用侧保�?HTTP |
| D3 | Windows 桌面�?| **独立、本机自用、免登录**；服务随软件启停，点开即用 |
| D4 | Docker / CLI 服务器版 | **必须有账�?*；继�?`0.0.0.0` 端口映射，文档强警告 |
| D5 | Android APP | **必须支持账密**，否则反代后无法访问 |
| D6 | 穿透文�?| 暂未指定产品；实现鉴权后补「通用反代 + Lucky 示例」即�?|
| D7 | 红线 | 不碰 `internal/douyin|bilibili` 解析与合并；版本强制迭代；验收前不公开发布 |

### 模型示意

```
Android APP ──HTTPS──�?Lucky(域名/证书) ──HTTP──�?Docker/CLI :18000
浏览�?Web  ──HTTPS──�?       �?                     �?                             �?                     ├─ 账密 Session
Windows 桌面 ─────────────────┴── �?127.0.0.1 ────�?└─ AUTH_MODE=off（桌面注入）
```

| 形�?| 监听 | 鉴权 | 说明 |
|---|---|---|---|
| Windows 桌面 | 建议默认 `127.0.0.1` | **off**（桌面启动器注入�?| 点开即用；与红线「本机随软件」一�?|
| Docker / CLI | `0.0.0.0`（保持现状） | **on** | 无登录则拒绝业务 API |
| 反代�?| Lucky �?容器/主机 | on | 用户在域名下用账号密�?|

---

## 1. 账密方案设计（定案原则）

### 1.1 账户模型（本期极简�?
- **单管理员账户**即可（家用下载器，不做多用户 RBAC�?- 字段：`username`（默认可建议 `admin`，但**密码无默�?*）、`password_hash`、`password_algo`、`updated_at`
- **禁止**出厂 `admin/admin` 或任何固定密�?
### 1.2 首次启动 / 初始化（避免「默认账密」坑�?
任选其一，建�?**A**�?
| 策略 | 行为 |
|---|---|
| **A. 首次强制设置密码（推荐）** | 未设置密码时：`/api/auth/setup` 仅允许一次；未完�?setup 前所有业�?API 返回 401 + `need_setup`；本�?桌面模式跳过 |
| B. 启动日志打印一次性初始化链接 | 仅回环可打开 setup �?|
| C. 随机初始密码写进本地文件 | 用户体验差，易丢 |

Windows 桌面：启动时若检测到「桌面模式」则 `auth_mode=off`，不�?setup�?
### 1.3 密码存储（必须）

```
auth.json  �?600，不�?git、不进镜像层�?{
  "username": "admin",
  "algo": "argon2id",          // 优先；备�?bcrypt
  "hash": "...",
  "params": { "time": 2, "memory": 65536, "threads": 2 },
  "updated_at": "..."
}
```

- **禁止**自定�?AES+固定盐存密码（避免重�?Cookie `deriveKey` 覆辙�?- 密码只走单向哈希；验证用恒定时间比较
- 修改密码：旧密码或本机文件权限；提供「本机重置」：�?`auth.json` 后需重新 setup（文档写明）

### 1.4 登录会话

| 客户�?| 机制 |
|---|---|
| Web | `POST /api/auth/login` �?HttpOnly Cookie（`SameSite=Lax`；反�?HTTPS 下可 `Secure`，注�?HTTP 调试用可配置�? 服务�?Session 存储（内�?+ 可选磁盘，重启后失效可接受�?|
| Android | 同一 login 接口；成功后�?**长期 API Token**（登录时签发，哈希落盘；可吊销）或复用 Cookie 方案（APP �?Cookie 罐） |
| 失败 | 限速：�?IP 例如 10 �?5 分钟；统一错误「用户名或密码错误�?|
| 退�?| `POST /api/auth/logout` �?Session / 吊销 Token |

**Android 建议�?* 登录成功签发 `access_token`（随�?32 字节，服务端存哈希），之�?Header `Authorization: Bearer <token>`；比依赖 Cookie 罐在�?WebView/网络库下更稳�?
### 1.5 哪些接口要保�?
| 类别 | 策略 |
|---|---|
| `/api/auth/login`、`/api/auth/setup`（未 setup 时） | 公开 |
| `/` 静态页、favicon | 公开（SPA 内再引导登录）或登录前只出登录壳 |
| 其余 `/api/**` | **全部需要登录�?* |
| 桌面 `AUTH_MODE=off` | 中间件直接放行（仅应出现在本机桌面） |

Cookie 设置、下载目录、update、ytdlp、文�?download/play/stream、logs —�?全部在保护伞下（解决审计 C1/H8/H7）�?
### 1.6 配置�?
| �?| 含义 |
|---|---|
| `AUTH_MODE` | `on` \| `off`（env 优先）；**Docker/CLI 默认 on**；桌面启动器可设 off |
| `BIND` | 可选；桌面�?`127.0.0.1`；Docker 保持默认全网�?|
| 配置路径 | `auth.json` 与现�?config 同目录策略，0600 |

**不引�?*「默认账号密码」配置项；不把密码写�?`config.json` �?Cookie 旁字段明文�?
---

## 2. 分端改造范�?
### 2.1 服务端（Go）�?Phase 1

| �?| 内容 |
|---|---|
| 依赖 | `golang.org/x/crypto/argon2`（或 bcrypt�?|
| 新增 | `internal/auth` �?`cmd/server/auth.go`：setup/login/logout/session/token |
| 中间�?| `r.Use(authMiddleware)` �?`AUTH_MODE`；白名单 auth 路由与静态资�?|
| 迁移 | 旧实例无 `auth.json` �?视为�?setup（服务器版）；桌面模�?off |
| 兼容 | 文档写明：升级后 APP/Web 需�?setup + 登录 |
| **不改** | 解析、合并、重试、下载策�?|
| 版本 | 服务器版升到下一个约定版本（实现时你定号，建�?`1.5.0`�?|

### 2.2 Web 控制�?�?Phase 2

- 登录�?/ 首次设置密码页（单文�?`static` 内，注意现有 index 体积，可�?`login` 片段�?- 401 时跳转登录；保存 Session Cookie
- 设置页：修改密码、退出登�?- 反代域名�?CORS：Lucky 一般是**同源反代**（域名直接打到应用）则无 CORS 问题；若跨端口需�?Origin 加入 allowlist

### 2.3 Android APP �?Phase 2

- 设置页：服务器地址（`https://域名`�? 用户�?+ 密码 �?登录
- 成功后持久化 token�?01 时清 token 回登�?设置
- 全部 `ApiService` �?`Authorization`
- 版本：`pubspec` version+1，`app-v` tag 独立
- **离线缓存**红线不破坏：未登录时不得用空数据覆盖已有缓存

### 2.4 Windows 桌面 �?Phase 1（轻�?
- 启动器注�?`AUTH_MODE=off`（或服务端检测「桌面窗口模式」）
- 可选：仅绑�?`127.0.0.1`（强烈建议，与「点开即用、无外部影响」一致）
- 无登�?UI

### 2.5 Docker / CLI / 文档 �?Phase 3

- compose **保持** `0.0.0.0` 映射（按你拍板）
- README / DOCKERHUB **强警�?*：公�?Lucky 必须 HTTPS + 强密码；禁止弱口令；建议防火墙只信家�?IP 或仅反代可达
- 补「Lucky 反代」章节：域名 �?后端 `http://内网IP:18000`；SSL �?Lucky 开；应用开 AUTH
- 说明：直�?`http://公网IP:18000` �?HTTPS 时密码可被窃�?�?**产品文档标红禁止**

---

## 3. 安全审计项与本方案的对应

| 审计�?| 鉴权落地�?|
|---|---|
| C1/C2/C3 无鉴�?+ Cookie 覆盖 | **消除**（需登录才能调） |
| H7/H8 更新、删文件、拉视频 | **消除** |
| H1/H2/H3 SSRF / Cookie 外带 | **仍在**，Phase 4 单独修（Hostname 匹配、不�?preview URL�?|
| M1 Cookie 静态密�?| 鉴权后风险降；Phase 4 仍建议改随机密钥文件 |
| 明文 HTTP | Lucky HTTPS 解决传输层；应用不自签证�?|

---

## 4. 分期路线�?
### Phase 0 �?决策 ✅（本文件）
- [x] 账密 + Lucky + Windows 免鉴�?+ Android 要登�? 
- [x] Docker 保持 0.0.0.0 + 强警�? 

### Phase 1 �?服务端账�?✅（feat-auth-login�?- [x] bcrypt 存储（argon2id �?bcrypt，同等单向哈希）
- [x] setup / login / logout / session �?Bearer
- [x] 中间�?+ `AUTH_MODE`
- [x] 桌面模式 off + 媒体 `?token=`
- [x] 登录限�?- [x] `go build` / `go vet` / 冒烟 curl 401/200
- [x] 版本号默�?1.5.0（发版仍 ldflags 注入�?
### Phase 2 �?Web + Android
- [x] Web 登录/设置密码�?`static/login.html`
- [x] Web 控制台退出登�?+ 登录状态徽�?- [ ] Web 改密（可在后续小版本补）
- [x] APP 登录、token�?01 不覆盖缓存、未登录强制登录�?- [x] 播放/下载 URL �?token
- [x] 本地 Ubuntu 实测 + APK 真机测通过
- [ ] Lucky/Hermes 域名 HTTPS 端到端（用户暂缓�?
### Phase 3 �?文档�?Docker 警告
- [x] README 鉴权 + 反代 HTTPS 警告 + Hermes/Lucky 场景�?- [x] 登录限速识�?X-Forwarded-For（反代下不锁死共�?IP�?- [ ] DOCKERHUB 单独文案是否同步（可后补�?- [ ] 升级迁移说明细化

### Phase 4 �?纵深（独�?PR�?- [ ] H1–H3 SSRF �?Cookie 域名匹配
- [ ] download_dir 基路径限�?- [ ] Cookie 随机密钥
- [ ] 会话落盘（重启免重登，可选）
- [ ] 依赖升级（需你确认）

### 已知限制（写给运维）
- 会话在内存：**重启服务�?Web/APP 均需重新登录**（密码不变）
- 反代勿再叠一层登录；保留 X-Forwarded-For
- 初始化请�?*最终对外域�?*下完成，避免 IP/域名混用 Cookie  

### 不做
- 出厂默认密码  
- 应用�?HTTPS  
- 多用户体�? 
- 改核心解�?合并  
- 未验�?push / �?Release  

---

## 5. 将来验收清单

1. Windows 桌面：双击即用，**无登录页**  
2. Docker 升级后：浏览器打开 �?**设置密码** �?登录 �?正常下载  
3. 未登录：`GET /api/tasks`、`POST /api/bilibili/cookie`、文件下载均 401  
4. Lucky `https://域名`：浏览器登录；APP 填域�?账密可创建任�? 
5. 错密码限速生效；改密后旧 token/会话失效策略符合文档  
6. `docker inspect` 无明文密�? 
7. 桌面�?Web 任务 `source` 隔离逻辑不回�? 
8. **你确认后**才允许推 main / �?tag  

---

## 6. 与你表述的对齐说�?
| 你的原话意图 | 计划如何落实 |
|---|---|
| Lucky 反代域名 + 账密登录 | D1/D2；Web �?APP 都走账密；HTTPS �?Lucky |
| CLI �?Docker 都加账密 | Phase 1 服务器强�?`AUTH_MODE` 默认 on |
| Android 不加就没法访�?| Phase 2 必做登录�?token |
| Windows 独立、点开即用 | 桌面注入 `AUTH_MODE=off`，可不出现登�?UI |
| 端口暴露正常、文档强警告 | compose 仍映�?0.0.0.0；README/DOCKERHUB 标红 |
| 先计划后开�?| 本文件确认后进入 Phase 1 |

---

*待你点头 Phase 1 范围后，再动代码�?

