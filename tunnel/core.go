// tunnel/tunnel.go
package tunnel

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings" // Нужен для зачистки пробелов
	"time"

	"github.com/armon/go-socks5"
	"golang.org/x/crypto/ssh"
)

// Сохраняем ссылки на активные сессии, чтобы их можно было закрыть извне
var (
	currentClient *ssh.Client
	socksListener net.Listener
)

// StartTunnel — это главная точка входа. Она настраивает SSH, открывает порт и запускает SOCKS5.
func StartTunnel(cfg *Config) string {
	log.Println("Запуск SSH туннеля...")

	// 1. Устанавливаем соединение с VPS
	client, err := Connect(cfg)
	if err != nil {
		return fmt.Sprintf("Ошибка SSH подключения: %v", err)
	}
	log.Println("Успешное SSH-подключение к серверу!")

	// 2. Настраиваем SOCKS5-сервер так, чтобы он перенаправлял запросы внутрь SSH сессии
	socksConf := &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return client.Dial(network, addr)
		},
	}
	server, err := socks5.New(socksConf)
	if err != nil {
		StopTunnel()
		return fmt.Sprintf("Ошибка инициализации SOCKS5: %v", err)
	}

	// 3. Начинаем слушать локальный порт (например, 127.0.0.1:9090)
	l, err := net.Listen("tcp", cfg.LocalProxyAddress)
	if err != nil {
		StopTunnel()
		return fmt.Sprintf("Ошибка портов (возможно, порт занят): %v", err)
	}
	SetListener(l)
	log.Printf("Локальный прокси-сервер запущен на %s", cfg.LocalProxyAddress)

	// 4. Запускаем обслуживание SOCKS5 сервера.
	// Метод Serve блокирует поток, поэтому на компьютере или в Android мы будем запускать StartTunnel в горутине.
	go func() {
		if err := server.Serve(l); err != nil {
			// Если туннель закрыли принудительно через StopTunnel(), Serve вернет ошибку закрытого листнера — это нормально
			log.Printf("SOCKS5 сервер остановлен: %v", err)
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

	log.Printf("Подключение к SSH %s...", cfg.ServerAddress)
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
	log.Println("Выключение туннеля и освобождение портов...")

	// 1. Закрываем локальный порт (сервер SOCKS5 сразу остановится)
	if socksListener != nil {
		socksListener.Close()
		socksListener = nil
	}

	// 2. Рвем соединение с сервером
	if currentClient != nil {
		currentClient.Close()
		currentClient = nil
	}

	log.Println("Туннель полностью остановлен. Порты свободны!")
}
