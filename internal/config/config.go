package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost            string
	DBPort            string
	DBName            string
	DBUser            string
	DBPassword        string
	ServerPort        string
	APIKey            string
	SanctionsJSONPath string
}

func Load() *Config {
	godotenv.Load()

	return &Config{
		DBHost:            getEnv("DB_HOST", "127.0.0.1"),
		DBPort:            getEnv("DB_PORT", "3306"),
		DBName:            getEnv("DB_NAME", "sanctions"),
		DBUser:            getEnv("DB_USER", "root"),
		DBPassword:        getEnv("DB_PASSWORD", ""),
		ServerPort:        getEnv("SERVER_PORT", "8080"),
		APIKey:            getEnv("API_KEY", ""),
		SanctionsJSONPath: getEnv("SANCTIONS_JSON_PATH", ""),
	}
}

func (c *Config) DSN() string {
	return c.DBUser + ":" + c.DBPassword + "@tcp(" + c.DBHost + ":" + c.DBPort + ")/" + c.DBName + "?charset=utf8mb4&parseTime=true&multiStatements=true"
}

// SeederDSN adds long timeouts for multi-hour bulk loads.
func (c *Config) SeederDSN() string {
	return c.DSN() + "&readTimeout=7200s&writeTimeout=7200s&timeout=60s"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
