package main

import (
	"encoding/json"
	"os"
)

// MonitorConfig описывает конфигурацию для одного монитора.
type MonitorConfig struct {
	Name      string `json:"Name"`      // Название монитора из системы (например, \\.\DISPLAY1)
	IsPrimary bool   `json:"IsPrimary"` // Является ли монитор основным
	Active    bool   `json:"Active"`    // Должно ли приложение запускаться на этом мониторе

	// Абсолютные координаты монитора в виртуальном рабочем столе Windows
	PositionX int `json:"PositionX"`
	PositionY int `json:"PositionY"`
	Width     int `json:"Width"`
	Height    int `json:"Height"`
}

// AppConfig описывает основную конфигурацию приложения.
type AppConfig struct {
	URL      string          `json:"URL"`
	Monitors []MonitorConfig `json:"Monitors"`
}

// getConfigPath возвращает путь к файлу config.json.
func getConfigPath() (string, error) {
	return "config.json", nil
}

// configExists проверяет, существует ли файл конфигурации.
func configExists() (bool, error) {
	path, err := getConfigPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// loadConfig загружает конфигурацию из файла.
func loadConfig() (*AppConfig, error) {
	path, err := getConfigPath()
	if err != nil {
		return nil, err
	}
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config AppConfig
	err = json.Unmarshal(file, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}
