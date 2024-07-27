package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	HTTPAuth struct {
		Enabled  bool   `json:"enabled"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"httpauth"`
	Addr string `json:"addr"`
}

var Conf Config

const ConfigFilePath = "./goprocess.json"

func ReadConfigFromJSON() error {
	// 尝试读取配置文件
	file, err := os.ReadFile(ConfigFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 如果配置文件不存在, 创建一个带有默认值的配置文件
			defaultConfig := Config{
				HTTPAuth: struct {
					Enabled  bool   `json:"enabled"`
					Username string `json:"username"`
					Password string `json:"password"`
				}{Enabled: false},
				Addr: "127.0.0.1:11315",
			}
			Conf = defaultConfig
			return writeConfigToJSON()
		}
		return err
	}

	// 解析配置文件
	if err := json.Unmarshal(file, &Conf); err != nil {
		return err
	}

	return nil
}

func writeConfigToJSON() error {
	// 序列化配置并写入文件
	data, err := json.MarshalIndent(Conf, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(ConfigFilePath, data, 0644)
}
