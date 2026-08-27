package logger

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var base *zap.Logger

func Init(level, format, service string) error {
	lvl, err := parseLevel(level)
	if err != nil {
		return err
	}

	encCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	if strings.EqualFold(format, "console") {
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encCfg)
	} else {
		encoder = zapcore.NewJSONEncoder(encCfg)
	}

	core := zapcore.NewCore(encoder, zapcore.Lock(zapcore.AddSync(stderr())), lvl)

	base = zap.New(core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
		zap.Fields(zap.String("service", service)),
	)
	return nil
}

func L() *zap.Logger {
	if base == nil {
		base = zap.NewNop()
	}
	return base
}

func S() *zap.SugaredLogger {
	return L().Sugar()
}

func Sync() {
	_ = L().Sync()
}

func parseLevel(level string) (zapcore.Level, error) {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(strings.ToLower(level))); err != nil {
		return lvl, fmt.Errorf("parse log level %q: %w", level, err)
	}
	return lvl, nil
}
