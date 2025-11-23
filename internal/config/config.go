package config

import (
	"fmt"
	"os"
	"strconv"
)

const (
	defaultHTTPPort    = "8080"
	defaultStorageType = "postgres"
	defaultDBHost      = "postgres"
	defaultDBPort      = "5432"
	defaultDBUser      = "reviewer"
	defaultDBPassword  = "reviewer"
	defaultDBName      = "reviewer"
	defaultDBSSLMode   = "disable"
	defaultDBMaxConns  = 4
)

type Config struct {
	HTTP    HTTPConfig
	Storage StorageConfig
}

type HTTPConfig struct {
	Addr string
}

type StorageConfig struct {
	Type     string
	Postgres PostgresConfig
}

type PostgresConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
	MaxConns int32
}

func (p PostgresConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", p.User, p.Password, p.Host, p.Port, p.DBName, p.SSLMode)
}

func Load() Config {
	port := getenvDefault("HTTP_PORT", defaultHTTPPort)

	storageType := getenvDefault("STORAGE_TYPE", defaultStorageType)
	pg := PostgresConfig{
		Host:     getenvDefault("DB_HOST", defaultDBHost),
		Port:     getenvDefault("DB_PORT", defaultDBPort),
		User:     getenvDefault("DB_USER", defaultDBUser),
		Password: getenvDefault("DB_PASSWORD", defaultDBPassword),
		DBName:   getenvDefault("DB_NAME", defaultDBName),
		SSLMode:  getenvDefault("DB_SSL_MODE", defaultDBSSLMode),
		MaxConns: int32(getenvInt("DB_MAX_CONNS", defaultDBMaxConns)),
	}

	return Config{
		HTTP: HTTPConfig{
			Addr: fmt.Sprintf(":%s", port),
		},
		Storage: StorageConfig{
			Type:     storageType,
			Postgres: pg,
		},
	}
}

func getenvDefault(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

func getenvInt(key string, def int) int {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return i
}
