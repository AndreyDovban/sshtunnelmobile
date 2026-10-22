// cmd/main.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Замените "ssheet" на имя вашего модуля из go.mod
	"ssh-tunnel-core/tunnel"
)

// Имитируем колбэк для логов, который в будущем прилетит из Kotlin
type ConsoleLogger struct{}

func (c ConsoleLogger) OnLogReceived(msg string) {
	fmt.Printf("[GO LOG] %s\n", msg)
}

func main() {
	fmt.Println("--- Старт тестового Go-приложения ---")
	// Загружаем конфигурацию из config.json
	cfg, err := tunnel.LoadConfig("config.json")
	if err != nil {
		fmt.Printf("Ошибка загрузки config.json: %v\n", err)
		return
	}

	// Переводим структуру в JSON-строку, имитируя поведение Kotlin
	// (импортируй пакет "encoding/json" в main.go, если компилятор ругнётся)
	jsonData, err := json.Marshal(cfg)
	if err != nil {
		fmt.Printf("Ошибка маршалинга в JSON: %v\n", err)
		return
	}

	// Запускаем туннель асинхронно
	go func() {
		status := tunnel.StartTunnel(string(jsonData))
		fmt.Printf("[MAIN] Статус запуска туннеля: %s\n", status)
	}()

	// Ожидаем системный сигнал прерывания (Ctrl+C), чтобы чисто закрыть порты
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	<-stopChan // Блокируем поток, пока не нажмут Ctrl+C

	fmt.Println("\n[MAIN] Получен сигнал остановки. Сворачиваемся...")
	tunnel.StopTunnel()

	// Даем немного времени на закрытие сетевых соединений
	time.Sleep(500 * time.Millisecond)
	fmt.Println("--- Программа успешно завершена ---")
}
