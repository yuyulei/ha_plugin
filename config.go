package ha

import (
	"encoding/json"
	"io/ioutil"
)

type Config struct {
	*Service         `json:"service"`
	*ConsulConfig    `json:"consul_config"`
	*HAManagerConfig `json:"ha_manager_config"`
}

func (c Config) String() string {
	return `Config: {Service: ` + c.Service.String() + `, ConsulConfig: ` + c.ConsulConfig.String() +
	`, HAManagerConfig: ` + c.HAManagerConfig.String() + `}`
}

func Load(filePath string, config *Config) error {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, config)
}
