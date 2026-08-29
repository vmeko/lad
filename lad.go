// Package lad provides small, opinionated helpers for building zap loggers.
//
// It follows two principles:
//
//  1. New builds a logger without touching zap globals.
//  2. InitGlobal is explicit about side effects (it replaces zap's global logger).
//
// File output uses lumberjack for log rotation.
package lad

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/DeRuina/timberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// type alias
type (
	Logger        = zap.Logger
	SugaredLogger = zap.SugaredLogger
	Level         = zapcore.Level
)

const (
	// DebugLevel logs are typically voluminous, and are usually disabled in
	// production.
	DebugLevel = zapcore.DebugLevel
	// InfoLevel is the default logging priority.
	InfoLevel = zapcore.InfoLevel
	// WarnLevel logs are more important than Info, but don't need individual
	// human review.
	WarnLevel = zapcore.WarnLevel
	// ErrorLevel logs are high-priority. If an application is running smoothly,
	// it shouldn't generate any error-level logs.
	ErrorLevel = zapcore.ErrorLevel
	// DPanicLevel logs are particularly important errors. In development the
	// logger panics after writing the message.
	DPanicLevel = zapcore.DPanicLevel
	// PanicLevel logs a message, then panics.
	PanicLevel = zapcore.PanicLevel
	// FatalLevel logs a message, then calls os.Exit(1).
	FatalLevel = zapcore.FatalLevel
)

// L returns the current global zap Logger (zap.L()).
func L() *Logger { return zap.L() }

// S returns the current global zap SugaredLogger (zap.S()).
func S() *SugaredLogger { return zap.S() }

// DefaultTimeFormat is the default timestamp layout used by encoders.
const DefaultTimeFormat = "2006-01-02 15:04:05.000"

// Option configures logger building behavior.
type Option func(*config) error

type config struct {
	coreBuilders []func(*config) (zapcore.Core, error)
	zapOpts      []zap.Option
	callerEncode zapcore.CallerEncoder
}

// WithZapOptions appends raw zap options to the logger being built.
func WithZapOptions(opts ...zap.Option) Option {
	return func(c *config) error {
		c.zapOpts = append(c.zapOpts, opts...)
		return nil
	}
}

// WithCaller enables caller annotations.
func WithCaller() Option {
	return func(c *config) error {
		c.zapOpts = append(c.zapOpts, zap.AddCaller())
		return nil
	}
}

// WithCallerSkip sets the number of stack frames to skip when reporting caller.
// Useful when you wrap this logger behind another logging facade.
func WithCallerSkip(skip int) Option {
	return func(c *config) error {
		if skip < 0 {
			return errors.New("lad: caller skip must be >= 0")
		}
		c.zapOpts = append(c.zapOpts, zap.AddCallerSkip(skip))
		return nil
	}
}

// WithCallerPathFrom configures caller rendering to start from the first
// occurrence of marker in the source file path.
//
// Example marker: "temekor" -> "temekor/application/service.go:93"
// If marker is not found, it falls back to zap's short caller format.
func WithCallerPathFrom(marker string) Option {
	return func(c *config) error {
		marker = strings.TrimSpace(marker)
		if marker == "" {
			return errors.New("lad: caller path marker cannot be empty")
		}
		c.callerEncode = callerEncoderFromMarker(marker)
		return nil
	}
}

// WithStacktrace enables stack traces at and above the given level.
func WithStacktrace(level Level) Option {
	return func(c *config) error {
		c.zapOpts = append(c.zapOpts, zap.AddStacktrace(level))
		return nil
	}
}

// ConsoleConfig controls console output.
type ConsoleConfig struct {
	Level      Level
	Colored    bool
	TimeFormat string   // Defaults to DefaultTimeFormat when empty.
	Output     *os.File // Defaults to os.Stdout when nil.
}

// WithConsole adds a console core to the logger.
func WithConsole(cc ConsoleConfig) Option {
	return func(c *config) error {
		c.coreBuilders = append(c.coreBuilders, func(cfg *config) (zapcore.Core, error) {
			out := cc.Output
			if out == nil {
				out = os.Stdout
			}

			encCfg := zap.NewProductionEncoderConfig()
			encCfg.EncodeTime = timeEncoder(orDefault(cc.TimeFormat, DefaultTimeFormat))
			encCfg.EncodeCaller = cfg.callerEncode

			if cc.Colored {
				encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
			} else {
				encCfg.EncodeLevel = zapcore.CapitalLevelEncoder
			}

			core := zapcore.NewCore(
				zapcore.NewConsoleEncoder(encCfg),
				zapcore.AddSync(out),
				cc.Level,
			)
			return core, nil
		})
		return nil
	}
}

// FileEncoding controls how file logs are encoded.
type FileEncoding string

const (
	// JSONEncoding writes logs as JSON (recommended for file output).
	JSONEncoding FileEncoding = "json"
	// ConsoleEncoding writes logs in console style (human-readable).
	ConsoleEncoding FileEncoding = "console"
)

// FileConfig controls rotating file output (powered by lumberjack).
type FileConfig struct {
	Level      zapcore.Level
	Filename   string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool

	Encoding   FileEncoding // Defaults to JSONEncoding when empty.
	TimeFormat string       // Defaults to DefaultTimeFormat when empty.
}

// WithFile adds a rotating file core to the logger.
func WithFile(fc FileConfig) Option {
	return func(c *config) error {
		c.coreBuilders = append(c.coreBuilders, func(cfg *config) (zapcore.Core, error) {
			if strings.TrimSpace(fc.Filename) == "" {
				return nil, errors.New("lad: FileConfig.Filename is required")
			}

			maxSize := fc.MaxSizeMB
			if maxSize <= 0 {
				maxSize = 100
			}

			encoding := fc.Encoding
			if encoding == "" {
				encoding = JSONEncoding
			}

			hook := &timberjack.Logger{
				Filename:   fc.Filename,
				MaxSize:    maxSize,
				MaxBackups: fc.MaxBackups,
				MaxAge:     fc.MaxAgeDays,
				Compress:   fc.Compress,
			}

			encCfg := zap.NewProductionEncoderConfig()
			encCfg.EncodeTime = timeEncoder(orDefault(fc.TimeFormat, DefaultTimeFormat))
			encCfg.EncodeLevel = zapcore.CapitalLevelEncoder
			encCfg.EncodeCaller = cfg.callerEncode

			var enc zapcore.Encoder
			switch encoding {
			case JSONEncoding:
				enc = zapcore.NewJSONEncoder(encCfg)
			case ConsoleEncoding:
				enc = zapcore.NewConsoleEncoder(encCfg)
			default:
				return nil, fmt.Errorf("lad: unknown FileEncoding %q", encoding)
			}

			core := zapcore.NewCore(
				enc,
				zapcore.AddSync(hook),
				fc.Level,
			)
			return core, nil
		})
		return nil
	}
}

// New builds a zap Logger with the given options.
// It does not modify zap's global logger.
func New(opts ...Option) (*Logger, error) {
	cfg := &config{
		callerEncode: zapcore.ShortCallerEncoder,
	}
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}

	// Default core if none provided: colored console at Debug level.
	if len(cfg.coreBuilders) == 0 {
		_ = WithConsole(ConsoleConfig{
			Level:      zap.DebugLevel,
			Colored:    true,
			TimeFormat: DefaultTimeFormat,
			Output:     os.Stdout,
		})(cfg)
	}

	cores := make([]zapcore.Core, 0, len(cfg.coreBuilders))
	for _, build := range cfg.coreBuilders {
		core, err := build(cfg)
		if err != nil {
			return nil, err
		}
		cores = append(cores, core)
	}

	core := zapcore.NewTee(cores...)
	return zap.New(core, cfg.zapOpts...), nil
}

// MustNew is like New but panics on error.
// This is intended for application startup code (e.g., main()).
func MustNew(opts ...Option) *Logger {
	l, err := New(opts...)
	if err != nil {
		panic(err)
	}
	return l
}

// InitGlobal builds a logger and replaces zap's global logger (zap.ReplaceGlobals).
func InitGlobal(opts ...Option) error {
	l, err := New(opts...)
	if err != nil {
		return err
	}
	zap.ReplaceGlobals(l)
	return nil
}

// MustInitGlobal is like InitGlobal but panics on error.
// This is intended for application startup code (e.g., main()).
func MustInitGlobal(opts ...Option) {
	err := InitGlobal(opts...)
	if err != nil {
		panic(err)
	}
}

// RedirectStdLog redirects the standard library's package-global logger (log.Print, etc.)
// to the supplied zap logger at InfoLevel.
//
// It returns a function that restores the original standard logger configuration.
func RedirectStdLog(l *Logger) func() {
	return zap.RedirectStdLog(l)
}

// RedirectStdLogAt is like RedirectStdLog but allows specifying the level.
func RedirectStdLogAt(l *Logger, level zapcore.Level) (func(), error) {
	return zap.RedirectStdLogAt(l, level)
}

// Sync flushes any buffered log entries.
// It ignores common errors produced when syncing stdout/stderr in some environments.
func Sync(l *Logger) error {
	if l == nil {
		return nil
	}
	err := l.Sync()
	if err == nil {
		return nil
	}
	if isIgnorableSyncErr(err) {
		return nil
	}
	return err
}

func timeEncoder(layout string) func(time.Time, zapcore.PrimitiveArrayEncoder) {
	return func(t time.Time, pae zapcore.PrimitiveArrayEncoder) {
		pae.AppendString(t.Format(layout))
	}
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func isIgnorableSyncErr(err error) bool {
	// Common in containers/terminals: "sync /dev/stderr: invalid argument"
	// Also happens on Windows; stdout/stderr sync is often unsupported there.
	if runtime.GOOS == "windows" {
		return true
	}
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTTY) || errors.Is(err, syscall.EBADF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "invalid argument") || strings.Contains(msg, "inappropriate ioctl") {
		return true
	}
	return false
}

func callerEncoderFromMarker(marker string) zapcore.CallerEncoder {
	marker = normalizePathPart(marker)
	return func(caller zapcore.EntryCaller, pae zapcore.PrimitiveArrayEncoder) {
		path := normalizePathPart(caller.File)
		if rel, ok := trimPathFromMarker(path, marker); ok {
			pae.AppendString(fmt.Sprintf("%s:%d", rel, caller.Line))
			return
		}
		zapcore.ShortCallerEncoder(caller, pae)
	}
}

func trimPathFromMarker(path, marker string) (string, bool) {
	if marker == "" {
		return "", false
	}
	if path == marker {
		return path, true
	}
	pivot := "/" + marker + "/"
	if idx := strings.Index(path, pivot); idx >= 0 {
		return path[idx+1:], true
	}
	if strings.HasPrefix(path, marker+"/") {
		return path, true
	}
	return "", false
}

func normalizePathPart(path string) string {
	return strings.Trim(strings.ReplaceAll(path, "\\", "/"), "/")
}
