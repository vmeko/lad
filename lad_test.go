package lad_test

import (
	"strconv"
	"testing"

	"github.com/vmeko/lad"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestInitGlobalSimple(t *testing.T) {
	lad.InitGlobal(
		lad.WithCaller(),
		lad.WithCallerPathFrom("temekor"),
	)

	defer func() { _ = lad.Sync(lad.L()) }()

	lad.L().Info("Log Init")
	_, err := strconv.Atoi("a")
	if err != nil {
		lad.L().Error("Conversion error", zap.Error(err))
	}
}

func TestMustInitGlobalConsole(t *testing.T) {
	lad.MustInitGlobal(
		lad.WithConsole(lad.ConsoleConfig{
			Level:   zap.DebugLevel,
			Colored: true,
		}),
		lad.WithCaller(),
		lad.WithStacktrace(zapcore.ErrorLevel),
	)

	defer func() { _ = lad.Sync(lad.L()) }()

	lad.L().Info("service started")
}

func TestMustInitGlobalFile(t *testing.T) {
	lad.MustInitGlobal(
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
		lad.WithStacktrace(zapcore.ErrorLevel),
	)

	defer func() { _ = lad.Sync(lad.L()) }()

	lad.S().Infow("request", "path", "/health", "ok", true)
}

func TestMustInitGlobal(t *testing.T) {
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
		lad.WithStacktrace(zapcore.ErrorLevel),
	)

	defer func() { _ = lad.Sync(lad.L()) }()

	lad.L().Info("service started")
	lad.S().Infow("request", "path", "/health", "ok", true)
}
