package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost           string
	DBPort           string
	DBName           string
	DBUser           string
	DBPassword       string
	ServerPort       string
	SanctionsJSONPath string
}

func Load() *Config {
	godotenv.Load()

	return &Config{
		DBHost:           getEnv("DB_HOST", "127.0.0.1"),
		DBPort:           getEnv("DB_PORT", "3306"),
		DBName:           getEnv("DB_NAME", "sanctions"),
		DBUser:           getEnv("DB_USER", "root"),
		DBPassword:       getEnv("DB_PASSWORD", ""),
		ServerPort:       getEnv("SERVER_PORT", "8080"),
		SanctionsJSONPath: getEnv("SANCTIONS_JSON_PATH", ""),
	}
}

func (c *Config) DSN() string {
	return c.DBUser + ":" + c.DBPassword + "@tcp(" + c.DBHost + ":" + c.DBPort + ")/" + c.DBName + "?charset=utf8mb4&parseTime=true&multiStatements=true"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
