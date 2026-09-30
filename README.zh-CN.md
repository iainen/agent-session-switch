# agent-session-switch

[English](README.md) | 简体中文

在**任意目录**里挑选 [Claude Code](https://claude.com/claude-code) 或 [Codex](https://github.com/openai/codex)
的会话，然后直接跳到会话当初启动时的目录并恢复它。

`claude --resume` 和 `codex resume` 只会列出当前目录下的会话。这个工具把所有会话集中列在一起，
并且可以实时预览对话内容。

![agent-session-switch：左侧是 Claude Code 会话列表，右侧是对话预览](docs/screenshot.png)

## 安装

```sh
go install github.com/iainen/agent-session-switch/cmd/agent-session-switch@latest
```

或者从源码构建：

```sh
git clone https://github.com/iainen/agent-session-switch
cd agent-session-switch
make build        # 生成 bin/agent-session-switch
```

需要 Go 1.26 或更高版本。每个 [release](https://github.com/iainen/agent-session-switch/releases)
都附带 Linux、macOS 和 Windows 的预编译二进制文件。

要恢复对应的会话，`claude` 和 / 或 `codex` 命令必须在你的 `PATH` 中。

## 使用

```sh
agent-session-switch               # 打开选择界面
agent-session-switch price         # 启动时在过滤框里预填 "price"
agent-session-switch -n 100        # 每个 agent 最多加载最近 100 个会话
agent-session-switch -- --model x  # -- 之后的参数原样传给 claude / codex
agent-session-switch -version
```

选中一个会话后，工具会切换到它原来的目录，并执行 `claude --resume <id>` 或 `codex resume <id>`。
工具会用 agent 替换自身进程，所以你的 shell 看到的就是 agent 本身。当前 shell 的目录不会被改变，
agent 退出后你仍回到原来的位置。

### 快捷键

| 按键 | 作用 |
| --- | --- |
| `j` / `k`，`↓` / `↑` | 向下 / 向上移动 |
| `g` / `G` | 跳到第一个 / 最后一个 |
| `PgUp` / `PgDn` | 一次移动 10 个 |
| `h` / `l`，`←` / `→` | 在 Claude 和 Codex 之间切换 |
| `Enter` | 打开会话（在回收站里是还原） |
| `/` | 按目录、标题或 id 过滤（用空格分隔多个词，需全部匹配）；`Esc` 回到导航 |
| `Ctrl-U` / `Ctrl-D`，鼠标滚轮 | 滚动预览 |
| `f` | 收藏 / 取消收藏 |
| `F` | 显示收藏夹（再按一次退出） |
| `d` | 移到回收站（会先确认） |
| `t` | 显示回收站（再按一次退出） |
| `r` | 从回收站还原 |
| `x` | 从回收站永久删除（会先确认） |
| `Tab` | 在 会话 → 收藏 → 回收站 之间循环 |
| `q`、`Esc`、`Ctrl-C` | 退出（在收藏夹或回收站里，`q` 和 `Esc` 是返回） |

过滤框获得焦点时，导航键不生效，所以可以在里面直接输入 `j` 或 `k`。
终端宽度小于 80 列时会隐藏预览。

## 数据存放位置

工具读取各 agent 已经写好的文件，并自己维护两个小文件。

| 路径 | 内容 | 本工具是否写入 |
| --- | --- | --- |
| `~/.claude/projects/*/*.jsonl` | Claude Code 会话 | 删除时移到回收站 |
| `~/.codex/sessions/*/*/*/rollout-*.jsonl` | Codex 会话 | 删除时移到回收站 |
| `~/.codex/session_index.jsonl` | Codex 会话名称 | 从不写入 |
| `~/.claude/session-favorites.json` | 你的收藏 | 会写入 |
| `~/.claude/session-trash/` | 已删除的会话，以及记录每个会话来源的 `.json` 文件 | 会写入 |

用 `d` 删除会话时，只是把文件**移动**到 `~/.claude/session-trash/`，只有用 `x` 才会真正删除。
还原时会把文件放回它原来的路径，并且不会覆盖已存在的文件。

需要了解的几点：

- 只会移动对话文件。各 agent 保存的其他与会话相关的数据（例如 Codex 自己的状态）不会被动，
  所以 agent 自己的界面里可能仍能看到已删除的会话。
- 每个 agent 只加载最新的 300 个会话（可用 `-n` 调整），所以更早的收藏会被保存，但不会显示。
- 不会列出 `~/.codex/archived_sessions/` 里的 Codex 会话。
- 本工具依赖各 agent 未公开文档的文件格式。如果某个 agent 改了格式，列表或预览可能出错，
  直到本工具更新为止。遇到这种情况请提 issue。
- 如果 `session-favorites.json` 存在但无法解析，收藏功能会变成只读，而不是把它覆盖掉。

## 开发

```sh
make check   # gofmt、go vet、带竞态检测的测试
make cover   # 覆盖率汇总
```

代码结构如下：

```
cmd/agent-session-switch/   命令行入口
internal/session/           查找并读取 Claude 和 Codex 的会话
internal/trash/             可恢复的回收站
internal/favorites/         收藏存储
internal/tui/               Bubble Tea 界面
```

测试使用临时的 home 目录，绝不会碰你真实的会话。详见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 许可证

[MIT](LICENSE)
