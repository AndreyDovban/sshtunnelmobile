// tunnel/tunnel.go
package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings" // Нужен для зачистки пробелов
	"time"

	"github.com/armon/go-socks5"
	"golang.org/x/crypto/ssh"
)

// Ссылки на активные сессии, чтобы их можно было закрыть извне
var (
	currentClient *ssh.Client
	socksListener net.Listener
	ctxCancel     context.CancelFunc
)

// Пинг сервера
func startKeepAlive(ctx context.Context, client *ssh.Client, interval int) {
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	const maxAliveAttempts = 7
	failedAttempts := 0

	sendPrintf("[Keep-Alive] Фоновый пинг запущен с интервалом %d сек. (Лимит пропущенных пингов: %d)", interval, maxAliveAttempts)

	for {
		select {
		case <-ticker.C:
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				failedAttempts++
				sendPrintf("[Keep-Alive] Предупреждение: Сервер не ответил на пинг (Сбой %d из %d): %v", failedAttempts, maxAliveAttempts, err)

				if failedAttempts >= maxAliveAttempts {
					sendPrintf("[Keep-Alive] Фатально: Связь потеряна окончательно (пропущено %d пингов). Закрываем туннель.", failedAttempts)
					StopTunnel()
					if appStatusListener != nil {
						appStatusListener.OnConnectionLost(err.Error())
					}
					return
				}
			} else {
				if failedAttempts > 0 {
					sendLog("[Keep-Alive] Связь успешно восстановилась, счетчик сбоев сброшен.")
				}
				failedAttempts = 0
				sendLog("[Keep-Alive] Пинг успешно отправлен и подтвержден сервером")
			}

		case <-ctx.Done():
			sendLog("[Keep-Alive] Получен сигнал остановки. Фоновый пинг завершен.")
			return
		}
	}
}

// StartTunnel — это главная точка входа. Она настраивает SSH, открывает порт и запускает SOCKS5.
func StartTunnel(configJSON string) string {
	var cfg Config
	err := json.Unmarshal([]byte(configJSON), &cfg)
	if err != nil {
		// Если Kotlin прислал кривой JSON, сразу пишем об этом в логи
		sendPrintf("Ошибка парсинга конфигурации JSON: %v", err)
		return fmt.Sprintf("Ошибка парсинга конфигурации JSON: %v", err)
	}

	sendLog("Запуск SSH туннеля...")

	// 1. Устанавливаем соединение с VPS
	client, err := Connect(&cfg)
	if err != nil {
		sendPrintf("Ошибка SSH подключения: %v", err)
		return fmt.Sprintf("Ошибка SSH подключения: %v", err)
	}
	sendLog("Успешное SSH-подключение к серверу!")

	// Инициализируем контекст отмены для фонового Keep-Alive пакетов
	// Бэкграунд-контекст оборачиваем в возможность отмены
	ctx, cancel := context.WithCancel(context.Background())
	ctxCancel = cancel // Запоминаем функцию отмены в глобальной переменной

	// Запускаем фоновый пинг сервера в отдельной горутине
	go startKeepAlive(ctx, client, cfg.ServerAliveInterval)

	// 2. Настраиваем SOCKS5-сервер так, чтобы он перенаправлял запросы внутрь SSH сессии
	socksConf := &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return client.Dial(network, addr)
		},
	}
	server, err := socks5.New(socksConf)
	if err != nil {
		StopTunnel()
		sendPrintf("Ошибка инициализации SOCKS5: %v", err)
		return fmt.Sprintf("Ошибка инициализации SOCKS5: %v", err)
	}

	// 3. Начинаем слушать локальный порт (например, 127.0.0.1:9090)
	l, err := net.Listen("tcp", cfg.LocalProxyAddress)
	if err != nil {
		StopTunnel()
		sendPrintf("Ошибка портов (возможно, порт занят): %v", err)
		return fmt.Sprintf("Ошибка портов (возможно, порт занят): %v", err)
	}
	SetListener(l)
	sendPrintf("Локальный прокси-сервер запущен на %s", cfg.LocalProxyAddress)

	// 4. Запускаем обслуживание SOCKS5 сервера.
	// Метод Serve блокирует поток, поэтому на компьютере или в Android мы будем запускать StartTunnel в горутине.
	go func() {
		if err := server.Serve(l); err != nil {
			// Если туннель закрыли принудительно через StopTunnel(), Serve вернет ошибку закрытого листнера — это нормально
			sendPrintf("SOCKS5 сервер остановлен: %v", err)
		}
	}()

	return "Туннель успешно запущен"
}

func Connect(cfg *Config) (*ssh.Client, error) {
	rawKey := strings.TrimSpace(cfg.KeyText)

	// Меняем пробел после заголовка на перенос строки \n
	if strings.HasPrefix(rawKey, "-----BEGIN OPENSSH PRIVATE KEY----- ") {
		rawKey = strings.Replace(rawKey, "-----BEGIN OPENSSH PRIVATE KEY----- ", "-----BEGIN OPENSSH PRIVATE KEY-----\n", 1)
	} else if strings.HasPrefix(rawKey, "-----BEGIN RSA PRIVATE KEY----- ") {
		rawKey = strings.Replace(rawKey, "-----BEGIN RSA PRIVATE KEY----- ", "-----BEGIN RSA PRIVATE KEY-----\n", 1)
	}

	// Меняем пробел перед подвалом на перенос строки \n
	if strings.HasSuffix(rawKey, " -----END OPENSSH PRIVATE KEY-----") {
		rawKey = strings.Replace(rawKey, " -----END OPENSSH PRIVATE KEY-----", "\n-----END OPENSSH PRIVATE KEY-----", 1)
	} else if strings.HasSuffix(rawKey, " -----END RSA PRIVATE KEY-----") {
		rawKey = strings.Replace(rawKey, " -----END RSA PRIVATE KEY-----", "\n-----END RSA PRIVATE KEY-----", 1)
	}

	signer, err := ssh.ParsePrivateKey([]byte(rawKey))
	if err != nil {
		return nil, err
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.SSHUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	sendPrintf("Подключение к SSH %s...", cfg.ServerAddress)
	client, err := ssh.Dial("tcp", cfg.ServerAddress, sshConfig)
	if err != nil {
		return nil, err
	}

	currentClient = client
	return client, nil
}

// Запоминаем запущенный SOCKS5-listener
func SetListener(l net.Listener) {
	socksListener = l
}

// StopTunnel мгновенно и чисто закрывает все сетевые ресурсы
func StopTunnel() {
	sendLog("Выключение туннеля и освобождение портов...")

	// 1. Посылаем сигнал отмены всем фоновым горутинам (включая Keep-Alive)
	if ctxCancel != nil {
		ctxCancel()
		ctxCancel = nil
	}

	// 2. Закрываем локальный порт SOCKS5
	if socksListener != nil {
		socksListener.Close()
		socksListener = nil
	}

	// 3. Рвем соединение с сервером
	if currentClient != nil {
		currentClient.Close()
		currentClient = nil
	}

	sendLog("Туннель полностью остановлен. Порты свободны!")
}
