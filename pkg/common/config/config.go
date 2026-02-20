// pkg/common/config/config.go
package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Service  ServiceConfig  `yaml:"service"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	Kafka    KafkaConfig    `yaml:"kafka"`
	Auth     AuthConfig     `yaml:"auth"`
	Gateway  GatewayConfig  `yaml:"gateway"`
	Mail     MailConfig     `yaml:"mail"`
	Jaeger   JaegerConfig   `yaml:"jaeger"`
}

type ServiceConfig struct {
	Name            string        `yaml:"name"`
	Port            int           `yaml:"port"`
	GRPCPort        int           `yaml:"grpc_port"`
	Environment     string        `yaml:"environment"`
	Version         string        `yaml:"version"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type KafkaConfig struct {
	Brokers []string `yaml:"brokers"`
	Topics  struct {
		EmailVerification string `yaml:"email_verification"`
		ServiceEvents     string `yaml:"service_events"`
		Logs              string `yaml:"logs"`
	} `yaml:"topics"`
}

type AuthConfig struct {
	JWTSecret           string        `yaml:"jwt_secret"`
	TokenExpiration     time.Duration `yaml:"token_expiration"`
	VerificationCodeTTL time.Duration `yaml:"verification_code_ttl"`
}

type GatewayConfig struct {
	HTTPPort  int `yaml:"http_port"`
	HTTPSPort int `yaml:"https_port"`
	WSPort    int `yaml:"ws_port"`
	UDPPort   int `yaml:"udp_port"`
	GRPCPort  int `yaml:"grpc_port"`
}

type MailConfig struct {
	SMTPHost     string `yaml:"smtp_host"`
	SMTPPort     int    `yaml:"smtp_port"`
	SMTPUser     string `yaml:"smtp_user"`
	SMTPPassword string `yaml:"smtp_password"`
	FromEmail    string `yaml:"from_email"`
}

type JaegerConfig struct {
	AgentHost string `yaml:"agent_host"`
	AgentPort int    `yaml:"agent_port"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
