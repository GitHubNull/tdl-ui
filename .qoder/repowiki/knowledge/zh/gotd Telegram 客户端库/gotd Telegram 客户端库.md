---
kind: external_dependency
name: gotd Telegram 客户端库
slug: gotd-telegram-client
category: external_dependency
category_hints:
    - auth_protocol
scope:
    - '**'
---

### Telegram MTProto 客户端
- **角色**：Telegram 官方推荐的 Go 客户端库，提供完整的 MTProto 协议实现
- **认证方式**：支持验证码登录、二维码扫码登录、Telegram Desktop 会话导入三种方式
- **连接管理**：ChatService 负责常驻连接，支持断线自动重建和错误恢复
- **数据模型**：Dialog、MediaItem、Selection 等类型定义与 Telegram API 对齐
- **代理支持**：SOCKS5 / HTTP 代理配置，适配不同网络环境
- **版本锁定**：v0.140.0，升级需关注 API 变更和 Breaking Changes