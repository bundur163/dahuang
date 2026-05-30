package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		WebPort    string `yaml:"web_port"`
		LogPath    string `yaml:"log_path"`
		LogMaxDays int    `yaml:"log_max_days"`
	} `yaml:"server"`
	RemoteAPI struct {
		BaseURL     string `yaml:"base_url"`
		ConfigAPI   string `yaml:"config_api"`
		QueueAPI    string `yaml:"queue_api"`
		FeedbackAPI string `yaml:"feedback_api"`
	} `yaml:"remote_api"`
	HTTP struct {
		Timeout    int `yaml:"timeout"`
		RetryCount int `yaml:"retry_count"`
	} `yaml:"http"`
}

var GlobalConfig Config

func LoadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, &GlobalConfig)
}
