package config

import (
	"github.com/spf13/viper"
)

const (
	PathUserHome = "$HOME"
)

var Configuration *Config

type Config struct {
	Server Server `json:"server" yaml:"server" mapstructure:"server"`
	//Version Version `json:"version" yaml:"version" mapstructure:"version"`
	HelpUrl string `json:"helpUrl" yaml:"helpUrl" mapstructure:"helpUrl"`
}

type Version struct {
	Agent  string `json:"agent" yaml:"agent" mapstructure:"agent"`
	Client string `json:"client" yaml:"client" mapstructure:"client"`
	Server string `json:"server" yaml:"server" mapstructure:"server"`
}

type Server struct {
	Port int `json:"port" yaml:"port" mapstructure:"port"`
}

func InitConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	//viper.AddConfigPath(ConfigPathUserHome)
	viper.AddConfigPath(PathUserHome)
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	err := viper.ReadInConfig()
	if err != nil {
		panic(err)
	}
	if err = viper.Unmarshal(&Configuration); err != nil {
		panic(err)
	}
}
