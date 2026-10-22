# Stella

## 编译

依赖 Go 1.27+。

构建 Windows/Linux 双平台产物到 `dist/`：

```sh
scripts/build.sh
```

产物命名 `dist/stella-{os}-{arch}[.exe]`（如 `stella-windows-amd64.exe`）。如需其他平台，在 `scripts/build.sh` 的 `targets` 中追加。

单独编译当前平台：

```sh
go build ./cmd/stella
```
