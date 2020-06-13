package config

import (
	"github.com/tkanos/gonfig"
)

type Configuration struct {
	Hostname   string
	Port       int
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
}

func GetConfig(configFile string) (Configuration, error) {
	config := Configuration{}
	err := gonfig.GetConf(configFile, &config)

	return config, err
}
