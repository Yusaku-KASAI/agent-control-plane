# Go の書き方

- エラーは `error` を返し、`fmt.Errorf("...: %w", err)` で文脈を足す。呼び出し側が判別するエラーは
  `var ErrXxx = errors.New(...)` として定義し、`errors.Is` で判定する。
- テストは標準の `testing` でテーブル駆動にする。インターフェースの偽物はテストファイルに手書きする。

```bash
make check-go   # golangci-lint、go mod tidy の差分、govulncheck、go test -race
make fmt        # 整形
```
