package appconfig

import (
	"flag"
	"log/slog"

	"github.com/kouhin/envflag"
)

type AppConfig struct {
	LogLevel      slog.Level
	ListenAddress string
	Hostname      string
}

func ParseConfig() *AppConfig {
	result := &AppConfig{}

	var logLevel string
	flag.StringVar(&logLevel, "log-level", "info", "One of: error, warn, info, debug")
	flag.StringVar(&result.ListenAddress, "listen-address", ":8443", "Address to listen on")
	flag.StringVar(&result.Hostname, "hostname", "", "Hostname to use in logs, if it needs to be different from OS-provided value")

	if err := envflag.Parse(); err != nil {
		panic(err)
	}

	switch logLevel {
	case "error":
		result.LogLevel = slog.LevelError
	case "warn":
		result.LogLevel = slog.LevelWarn
	case "info":
		result.LogLevel = slog.LevelInfo
	case "debug":
		result.LogLevel = slog.LevelDebug
	default:
		panic("unknown log level: " + logLevel)
	}

	return result
}
