package version

type Version struct {
	Agent  string `json:"agent" yaml:"agent" mapstructure:"agent"`
	Client string `json:"client" yaml:"client" mapstructure:"client"`
	Server string `json:"server" yaml:"server" mapstructure:"server"`
}
