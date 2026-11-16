package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type DatabaseConfig struct {
	Driver   string `env:"DRIVER" envDefault:"mongodb+srv"`
	Host     string `env:"HOST"`
	Port     string `env:"PORT" envDefault:"27017"`
	Name     string `env:"NAME" envDefault:"core"`
	User     string `env:"USER"`
	Password string `env:"PASSWORD"`
	// Query string da URI, como directConnection=true para o Mongo do compose
	// acessado de fora da rede do Docker.
	Params string `env:"PARAMS"`
}

func (d DatabaseConfig) URI() string {
	host := d.Host
	if !strings.HasSuffix(d.Driver, "+srv") && d.Port != "" {
		host = fmt.Sprintf("%s:%s", d.Host, d.Port)
	}

	u := url.URL{Scheme: d.Driver, Host: host, Path: "/", RawQuery: d.Params}
	if d.User != "" {
		u.User = url.UserPassword(d.User, d.Password)
	}
	return u.String()
}

type ServerConfig struct {
	Port string `env:"PORT" envDefault:"8080"`
	// Cabeçalho com o IP do cliente, posto pelo proxy da frente. Vazio, o IP é
	// o da conexão; definido, só vale quando a conexão vem de rede privada ou
	// loopback, para um cliente direto não forjar o próprio IP.
	ProxyHeader string `env:"PROXY_HEADER"`
}

type AuthConfig struct {
	SessionTTL time.Duration `env:"SESSION_TTL" envDefault:"2160h"`
}

// O padrão é o servidor de dev do app; fora dele, CORS_ORIGINS precisa ser definida.
type CORSConfig struct {
	Origins []string `env:"ORIGINS" envDefault:"http://localhost:5173"`
}

type Config struct {
	DB     DatabaseConfig `envPrefix:"DB_"`
	Server ServerConfig   `envPrefix:"SERVER_"`
	Auth   AuthConfig     `envPrefix:"AUTH_"`
	CORS   CORSConfig     `envPrefix:"CORS_"`
}

func LoadConfig() *Config {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Fatalf("erro ao carregar arquivo .env: %v", err)
	}

	var cfg Config

	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("erro ao carregar variáveis de ambiente: %v", err)
	}

	return &cfg
}
