package logger

import "github.com/spf13/viper"

type Config struct {
	Logger Logger `mapstructure:"logger"`
	Cfg    *viper.Viper
}

type GoBaseToolsConfig interface {
	NewConfig(optionalPath *string) (*Config, error)
	SaveConfig(config *Config, filePath string) error
	GetDefaultConfig() Config
}
