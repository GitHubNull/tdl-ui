---
kind: external_dependency
name: modernc SQLite 嵌入式数据库
slug: modernc-sqlite
category: external_dependency
category_hints:
    - migration_status
scope:
    - '**'
---

### 纯 Go SQLite 实现
- **角色**：计划中的下载记录持久化存储，替代当前的 JSON 文件存储
- **优势**：无 CGO 依赖，Windows 构建无需额外工具链，支持关系型查询和事务
- **迁移策略**：从 `tasks.json` 迁移到 SQLite 表结构，保持向后兼容
- **当前状态**：方案已制定（`doc/todo/sqlite-yaml-append-plan.md`），尚未实施
- **依赖选择**：选用 `modernc.org/sqlite` 而非 `mattn/go-sqlite3`，避免 CGO 依赖
- **版本锁定**：v1.54.0，升级需验证 SQL 兼容性和性能影响