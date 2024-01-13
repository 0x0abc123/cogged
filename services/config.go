package services

import (
	"os"
	"path/filepath"
	"encoding/json"
	"cogged/log"
)

/*
	Config is a struct that holds all the configuration for the application
	create config.json file in the root directory of the application with the following flat format:
	{
		"db.host": "localhost",
		"db.port": "9080"
		"log.level": "info",
		"log.file": "cogged.log",
		"secret.mode": "default",
		"auth.tokenexpiry": "600"
	}
*/

const CONFIG_FILE_NAME string = "cogged.conf.json"

type Config map[string]string

func (c *Config) Get(key string) string {
	return (*c)[key]
}

func LoadConfig(cliValue string) *Config {
	// CLI flag overrides other places
	configFilePath := cliValue 
	// try getting path from envionment variable if not from CLI flag
	if configFilePath == "" {
		configFilePath = os.Getenv("COGGED_CONFIG_FILE")
		if configFilePath == "" {
			// try current working directory
			configFilePath = workingDirectoryConfigPath()
			if !statFile(configFilePath) {
				// try exe directory
				configFilePath = exeDirectoryConfigPath()
				if !statFile(configFilePath) {
					panic("Could not load config file")
					return nil
				}
			}
			configFilePath = configFilePath
		}
	}
	confFile, err := os.ReadFile(configFilePath)
	if err != nil {
    	panic(err)
	}

	var confData Config
	if err := json.Unmarshal(confFile, &confData); err != nil {
        panic(err)
    }
	return &confData
}

func statFile(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func exeDirectoryConfigPath() string {
	exePath, err := os.Executable()
	if err != nil {
		log.Error("Error getting executable path:", err)
		return ""
	}
	// Get the directory of the executable file
	exeDir := filepath.Dir(exePath)
	return exeDir + "/" + CONFIG_FILE_NAME
}

func workingDirectoryConfigPath() string {
	currentDir, err := os.Getwd()
	if err != nil {
		log.Error("Error getting current working directory:", err)
		return ""
	}
	return currentDir + "/" + CONFIG_FILE_NAME
}