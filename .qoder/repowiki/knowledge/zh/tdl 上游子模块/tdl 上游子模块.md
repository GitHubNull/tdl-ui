---
kind: external_dependency
name: tdl 上游子模块
slug: tdl-upstream
category: external_dependency
category_hints:
    - client_constraint
scope:
    - '**'
---

### tdl 上游依赖管理
- **角色**：Telegram 下载核心引擎，通过 Git 子模块方式引入，禁止直接修改
- **引入方式**：`go.mod replace` 指向本地 `../ref/tdl`，实际版本由子模块 commit 决定
- **子模块约束**：ARC-05 规则，仅允许同步上游变更，commit 变更需随主仓库一起提交
- **依赖范围**：`github.com/iyear/tdl` 和 `github.com/iyear/tdl/core` 两个模块均被替换
- **构建要求**：构建前必须执行 `git submodule update --init` 获取最新子模块代码