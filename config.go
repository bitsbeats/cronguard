package main

import (
	"os"

	"gopkg.in/yaml.v2"
)

type (
	// Config holds the optional and global configuration
	Config struct {
		SentryDSN string `yaml:"sentry_dsn"`
	}
)

// ParseConfig loads the Configfile if there is one or uses defaults
func ParseConfig() *Config {
	c := Config{}
	file, err := open("cronguard.yml", "cronguard.yaml", "/etc/cronguard.yml", "/etc/cronguard.yaml")
	if err != nil {
		return &c
	}
	_ = yaml.NewDecoder(file).Decode(&c)
	return &c
}

func open(files ...string) (*os.File, error) {
	err := error(nil)
	for _, file := range files {
		file, err := os.Open(file)
		if err == nil {
			return file, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return nil, err
}
