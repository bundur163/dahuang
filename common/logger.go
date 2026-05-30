package common

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/yaml.v3"
)

func InitLogger(path string) *zap.Logger {
	_ = os.MkdirAll(path, 0o755)

	keepDays := 14
	if data, err := os.ReadFile("/opt/msg-service/config.yaml"); err == nil {
		var cfg struct {
			Server struct {
				LogMaxDays int `yaml:"log_max_days"`
			} `yaml:"server"`
		}
		if err := yaml.Unmarshal(data, &cfg); err == nil && cfg.Server.LogMaxDays > 0 {
			keepDays = cfg.Server.LogMaxDays
		}
	}

	fileWriter := &lumberjack.Logger{
		Filename:   filepath.Join(path, "service.log"),
		MaxSize:    100,
		MaxBackups: 10,
		MaxAge:     keepDays,
		Compress:   true,
	}

	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{"stdout", "service.log"}
	cfg.ErrorOutputPaths = []string{"stderr", "service.log"}
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(cfg.EncoderConfig),
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(fileWriter), zapcore.AddSync(os.Stdout)),
		cfg.Level,
	)

	logger := zap.New(core)
	return logger
}
