// tunnel/config.go
package tunnel

import (
	"encoding/json"
	"os"
)

type Config struct {
	ServerAddress       string `json:"server_address"`
	SSHUser             string `json:"ssh_user"`
	KeyText             string `json:"key_text"`
	LocalProxyAddress   string `json:"local_proxy_address"`
	ServerAliveInterval int    `json:"server_alive_interval"`
}

// Функция загрузки настроек
func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Функция для перезаписи файла config.json новыми параметрами с экрана
func SaveConfig(path string, cfg *Config) error {
	// Маршалим структуру в красивый отформатированный JSON байт-массив
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}

	// Перезаписываем файл на диске с правами на чтение/запись
	return os.WriteFile(path, data, 0644)
}
