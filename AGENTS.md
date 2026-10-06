# AI Rules

## 本地测试与验证

- 修改前先读 `.agentdocs/index.md`；文档、注释与沟通使用中文。
- Go 变更运行 `gofmt`、对应包测试、`go vet ./...` 和 `go test ./...`。
- 新增或变更功能补充现有 Go testing 单元及集成测试，不引入测试框架。
- 页面脚本修改运行 `node --check`、`node --test tools/market-business-ui.test.cjs` 和 `internal/entry/webadmin` 测试。
- 主程序按 `.github/workflows` 编译 Linux amd64、CGO_ENABLED=0 到 `dist/robot`。
- 若修改独立模块 `tools/deploy-launcher`，在该目录额外运行 `go vet ./...` 和 `go test ./...`。

Before any VM, deploy, or debug task, read:

- `doc/vm.md`

Must follow:

- Read docs as UTF-8.
- Do not call Chinese text garbled.
- Use Python `paramiko` for VM SSH, upload, and remote commands.
- Do not use PowerShell `ssh` or `scp` for VM work.
- Do not restore VM snapshots unless the user asks.
- Deploy only after recording the git commit and backing up `/root/robot`.
- After deploy, check process, ports, and logs.

Fast VM card:

- VM: `192.168.200.131`
- SSH: `root / 123456`
- Web: `http://192.168.200.131:8112`
- Web password: `twadmin`
- robot API: `8111`
- game: `10011`
- auction: `30803`
- point: `30603`
- deployment root: `/root` only
- robot: `/root/robot`
- config: `/root/config`
- main config: `/root/config/conf/config.ini`
- runtime logs: `/root/config/logs/`
- templates, keys, PVF, state, and temporary files: `/root/config/templates/`, `/root/config/keys/`, `/root/config/pvf/`, `/root/config/state/`, `/root/config/tmp/`

Every deployment moves the complete `/root/config` to `/root/config.bak.<timestamp>`, keeps only the newest three config backups, creates a fresh `/root/config`, and lets robot regenerate all files. Do not migrate individual files automatically; users recover anything needed from the backup.

All Robot-owned generated files stay below `/root/config/{conf,templates,keys,pvf,state,logs,tmp}`. Game RSA files and Auction/Point `iteminfo.dat` are external integration files/copies, not alternate deployment roots. A normal Restart only restarts `/root/robot` and must not move, delete, or recreate `/root/config`.

Start robot with the bounded stdout sink:

```sh
mkdir -p /root/config/logs
nohup sh -c '/root/robot 2>&1 | /root/robot --bounded-log-sink /root/config/logs/stdout.log' >/dev/null 2>/root/config/logs/start_error.log &
```
