package config

import (
	"github.com/spf13/viper"
)

const (
	PathUserHome = "$HOME"
)

var Configuration *Config

type Config struct {
	Server  Server  `json:"server" yaml:"server" mapstructure:"server"`
	Agent   Agent   `json:"agent" yaml:"agent" mapstructure:"agent"`
	Client  Client  `json:"client" yaml:"client" mapstructure:"client"`
	Version Version `json:"version" yaml:"version" mapstructure:"version"`
	HelpUrl string  `json:"helpUrl" yaml:"helpUrl" mapstructure:"helpUrl"`
}

type Version struct {
	Agent  string `json:"agent" yaml:"agent" mapstructure:"agent"`
	Client string `json:"client" yaml:"client" mapstructure:"client"`
	Server string `json:"server" yaml:"server" mapstructure:"server"`
}

type Server struct {
	Port   int    `json:"port" yaml:"port" mapstructure:"port"`
	Host   string `json:"host" yaml:"host" mapstructure:"host"`
	Schema Schema `json:"schema" yaml:"schema" mapstructure:"schema"`
}

type Agent struct {
	Port int    `json:"port" yaml:"port" mapstructure:"port"`
	Host string `json:"host" yaml:"host" mapstructure:"host"`
}

type Client struct {
	Port int    `json:"port" yaml:"port" mapstructure:"port"`
	Host string `json:"host" yaml:"host" mapstructure:"host"`
}

type Schema struct {
	Http string `json:"http" yaml:"http" mapstructure:"http"`
	Ws   string `json:"ws" yaml:"ws" mapstructure:"ws"`
}

func InitConfig() {
	viper.SetConfigName("t2t-config")
	viper.SetConfigType("yaml")
	//viper.AddConfigPath(PathUserHome)
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
