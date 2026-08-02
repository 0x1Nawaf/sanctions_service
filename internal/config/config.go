package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost            string
	DBPort            string
	DBName            string
	DBUser            string
	DBPassword        string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
	DBReadTimeout     time.Duration
	DBWriteTimeout    time.Duration
	ServerPort        string
	APIKey            string
	SanctionsJSONPath string
	ScreenUseLike     bool
	EnablePprof       bool
}

func Load() *Config {
	godotenv.Load()

	return &Config{
		DBHost:            getEnv("DB_HOST", "127.0.0.1"),
		DBPort:            getEnv("DB_PORT", "3306"),
		DBName:            getEnv("DB_NAME", "sanctions"),
		DBUser:            getEnv("DB_USER", "root"),
		DBPassword:        getEnv("DB_PASSWORD", ""),
		DBMaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 50),
		DBMaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 25),
		DBConnMaxLifetime: time.Duration(getEnvInt("DB_CONN_MAX_LIFETIME_MIN", 5)) * time.Minute,
		DBReadTimeout:     time.Duration(getEnvInt("DB_READ_TIMEOUT_SEC", 30)) * time.Second,
		DBWriteTimeout:    time.Duration(getEnvInt("DB_WRITE_TIMEOUT_SEC", 30)) * time.Second,
		ServerPort:        getEnv("SERVER_PORT", "8080"),
		APIKey:            getEnv("API_KEY", ""),
		SanctionsJSONPath: getEnv("SANCTIONS_JSON_PATH", ""),
		ScreenUseLike:     getEnvBool("SCREEN_USE_LIKE_FALLBACK", false),
		EnablePprof:       getEnvBool("ENABLE_PPROF", false),
	}
}

func (c *Config) DSN() string {
	dsn := c.DBUser + ":" + c.DBPassword + "@tcp(" + c.DBHost + ":" + c.DBPort + ")/" + c.DBName +
		"?charset=utf8mb4&parseTime=true&multiStatements=true"
	if c.DBReadTimeout > 0 {
		dsn += "&readTimeout=" + durationParam(c.DBReadTimeout)
	}
	if c.DBWriteTimeout > 0 {
		dsn += "&writeTimeout=" + durationParam(c.DBWriteTimeout)
	}
	return dsn
}

// SeederDSN adds long timeouts for multi-hour bulk loads.
func (c *Config) SeederDSN() string {
	return c.DSN() + "&readTimeout=7200s&writeTimeout=7200s&timeout=60s"
}

func durationParam(d time.Duration) string {
	if d%(time.Second) == 0 {
		return strconv.Itoa(int(d/time.Second)) + "s"
	}
	return d.String()
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
