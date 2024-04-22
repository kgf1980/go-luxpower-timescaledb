package config

import (
	"errors"
	"net/url"
	"os"
)

type Config struct {
	AccountName   string
	Password      string
	StationNumber string
	BaseUrl       *url.URL
	DatabaseUrl   string
}

func NewConfig() (*Config, error) {
	var (
		cfg   Config
		found bool
	)

	cfg.AccountName, found = os.LookupEnv("ACCOUNT_NAME")
	if !found || cfg.AccountName == "" {
		return &Config{}, errors.New("account name not set")
	}

	cfg.Password, found = os.LookupEnv("ACCOUNT_PASSWORD")
	if !found || cfg.Password == "" {
		return &Config{}, errors.New("account password not set")
	}

	cfg.StationNumber, found = os.LookupEnv("STATION_NUMBER")
	if !found || cfg.StationNumber == "" {
		return &Config{}, errors.New("station number not set")
	}

	baseUrl, found := os.LookupEnv("LUXPOWER_URL")
	if !found || baseUrl == "" {
		return &Config{}, errors.New("base url not set")
	}

	url, err := url.Parse(baseUrl)
	if err != nil {
		return &Config{}, err
	}

	cfg.BaseUrl = url

	cfg.DatabaseUrl, found = os.LookupEnv("DATABASE_URL")
	if !found || cfg.DatabaseUrl == "" {
		return &Config{}, errors.New("database url not set")
	}

	return &cfg, nil
}
