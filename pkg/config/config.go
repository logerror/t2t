package config

import (
	"github.com/logerror/easylog"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const (
	PathUserHome = "$HOME"
)

var Configuration *Config

type Config struct {
	Server  Server     `json:"server" yaml:"server" mapstructure:"server"`
	HelpUrl string     `json:"helpUrl" yaml:"helpUrl" mapstructure:"helpUrl"`
	Auth    AuthConfig `json:"auth" yaml:"auth" mapstructure:"auth"`
}

type Version struct {
	Agent  string `json:"agent" yaml:"agent" mapstructure:"agent"`
	Client string `json:"client" yaml:"client" mapstructure:"client"`
	Server string `json:"server" yaml:"server" mapstructure:"server"`
}

type Server struct {
	Port int `json:"port" yaml:"port" mapstructure:"port"`
}

// 新增 AuthConfig 结构体
// 支持多个账号密码
type AuthConfig struct {
	Mode            string           `json:"mode" yaml:"mode" mapstructure:"mode"`
	Users           []UserCredential `json:"users" yaml:"users" mapstructure:"users"`
	LDAPURL         string           `json:"ldapUrl" yaml:"ldapUrl" mapstructure:"ldapUrl"`
	LDAPBaseDN      string           `json:"ldapBaseDN" yaml:"ldapBaseDN" mapstructure:"ldapBaseDN"`
	LDAPServiceUser string           `json:"ldapServiceUser" yaml:"ldapServiceUser" mapstructure:"ldapServiceUser"`
	LDAPServicePass string           `json:"ldapServicePass" yaml:"ldapServicePass" mapstructure:"ldapServicePass"`
}

type UserCredential struct {
	Username string `json:"username" yaml:"username" mapstructure:"username"`
	Password string `json:"password" yaml:"password" mapstructure:"password"`
}

func InitConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
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
	easylog.Info("Configuration loaded successfully", zap.String("configFilePath", viper.ConfigFileUsed()))
}
