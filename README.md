# lad

`lad` is a small helper package for building [zap](https://pkg.go.dev/go.uber.org/zap) loggers with a clean, explicit API.

It supports:

- Console logging (optionally colored)
- Rotating file logging via `lumberjack`
- Functional options for configuration
- Explicit global initialization (`InitGlobal`) vs local-only logger creation (`New`)
- Safe `Sync` helper
- Optional standard library `log` redirection helpers

---

## Installation

```bash
go get github.com/temekor/lad
```

---

## Quick Start (Global Logger)

Use a global logger when your application primarily logs through `zap.L()` / `zap.S()`.

### Minimal

The quickest way to log to the terminal using the global logger:

```go
lad.MustInitGlobal()
defer func() { _ = lad.Sync(lad.L()) }()

lad.L().Info("hello")
```

### Configured Example

```go
package main

import (
  "go.uber.org/zap"
  "go.uber.org/zap/zapcore"

  "github.com/temekor/lad"
)

func main() {
  lad.MustInitGlobal(
    lad.WithConsole(lad.ConsoleConfig{
      Level:   zap.DebugLevel,
      Colored: true,
    }),
    lad.WithFile(lad.FileConfig{
      Level:      zap.InfoLevel,
      Filename:   "./logs/app.log",
      MaxSizeMB:  200,
      MaxBackups: 10,
      MaxAgeDays: 30,
      Compress:   true,
      Encoding:   lad.JSONEncoding, // default is JSONEncoding
    }),
    lad.WithCaller(),
    lad.WithCallerPathFrom("temekor"), // caller: temekor/xxx/yyy.go:line
    lad.WithStacktrace(zapcore.ErrorLevel),
  )

  defer func() { _ = lad.Sync(lad.L()) }()

  lad.L().Info("service started")
  lad.S().Infow("request", "path", "/health", "ok", true)
}
```

### Defaults

If you call `InitGlobal()` (or `New()`) without adding any cores, `lad` falls back to:

- Console output
- Debug level
- Colored levels
- Timestamp format: `2006-01-02 15:04:05.000`

---

## Local Logger (No Global Side Effects)

Use `New` when you want a logger instance without changing zap globals. This is recommended for:

- Unit tests
- Multiple loggers in one process (different modules writing to different files)
- Libraries that should not modify global state

```go
logger, err := lad.New(
  lad.WithConsole(lad.ConsoleConfig{
    Level: zap.InfoLevel,
  }),
)
if err != nil {
  panic(err)
}
defer func() { _ = lad.Sync(logger) }()

logger.Info("local logger only")
```

---

## Rotating File Output

File rotation uses `lumberjack` under the hood.

```go
lad.WithFile(lad.FileConfig{
  Level:      zap.InfoLevel,
  Filename:   "./logs/app.log",
  MaxSizeMB:  200, // rotate after 200MB
  MaxBackups: 10,  // keep 10 backups
  MaxAgeDays: 30,  // keep 30 days
  Compress:   true,
  Encoding:   lad.JSONEncoding, // recommended for file output
})
```

### File Encoding

- `JSONEncoding` (default): structured JSON logs; best for ingestion by log systems.
- `ConsoleEncoding`: human-readable output in the file.

---

## Redirect Standard Library `log` (Optional)

If you still have code that calls `log.Print` / `log.Printf`, you can redirect it to zap.

> This uses zap's built-in redirection helpers and returns an undo function.

```go
package main

import (
  "log"

  "go.uber.org/zap/zapcore"
  "github.com/temekor/lad"
)

func main() {
  lad.MustInitGlobal()

  undo, err := lad.RedirectStdLogAt(lad.L(), zapcore.InfoLevel)
  if err != nil {
    panic(err)
  }
  defer undo()

  // Now log.Print writes into zap.
  log.Print("hello from stdlib log")
}
```

---

## Flushing Logs (Sync)

Always call `Sync` before process exit to flush buffered logs.

```go
defer func() { _ = lad.Sync(lad.L()) }()
```

`lad.Sync` ignores common `Sync()` errors produced by stdout/stderr in some environments.

---

## API Summary

### Logger creation

- `New(opts ...Option) (*zap.Logger, error)`
- `MustNew(opts ...Option) *zap.Logger`
- `InitGlobal(opts ...Option) (*zap.Logger, error)`
- `MustInitGlobal(opts ...Option) *zap.Logger`

### Global access

- `L() *zap.Logger`
- `S() *zap.SugaredLogger`

### Outputs

- `WithConsole(ConsoleConfig)`
- `WithFile(FileConfig)`

### zap options

- `WithCaller()`
- `WithCallerPathFrom(marker string)` (e.g. `marker="temekor"` -> `temekor/path/to/file.go:line`)
- `WithCallerSkip(skip int)`
- `WithStacktrace(level zapcore.Level)`
- `WithZapOptions(opts ...zap.Option)`

### Utilities

- `Sync(*zap.Logger) error`
- `RedirectStdLog(*zap.Logger) func()`
- `RedirectStdLogAt(*zap.Logger, zapcore.Level) (func(), error)`

---

## License

MIT (or your preferred license)

## AI Docs

- Repo-specific AI context: `.ai/PROJECT.md`
