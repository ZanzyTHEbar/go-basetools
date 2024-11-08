package logger

import (
	"log/slog"
	"os"

	"github.com/labstack/gommon/log"
)

type Logger struct {
	Style string `mapstructure:"style"`
	Level string `mapstructure:"level"`
}

var LoggerLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

var LoggerStyles = map[string]bool{
	"json": true,
	"text": true,
	"dev":  true,
}

// Set up the logger based on the configuration
// Must be called before server is started
func InitLogger(config *Config) {

	var handler slog.Handler

	logLevel, ok := LoggerLevels[string(config.Logger.Level)]

	if !ok {
		log.Debugf("Invalid log level: %s, Using default value INFO", config.Logger.Level)
		logLevel = slog.LevelInfo
	}

	switch config.Logger.Style {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	case "text":
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	case "dev":
		handler = NewDevHandler(os.Stdout, logLevel)
	default:
		log.Debugf("Invalid log style: %s, Using default value JSON", config.Logger.Style)
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
