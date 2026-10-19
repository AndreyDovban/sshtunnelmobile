// tunnel/logger.go
package tunnel

import (
	"fmt"
	"log"
)

// Logger — этот интерфейс gomobile автоматически превратит в Java-интерфейс.
// Kotlin реализует его, чтобы получать строчки логов на своей стороне.
type Logger interface {
	OnLogReceived(msg string)
}

// Глобальная переменная внутри пакета tunnel, куда мы сохраним логгер из Kotlin
var appLogger Logger

// SetLogger позволяет передать логгер в пакет tunnel
func SetLogger(l Logger) {
	appLogger = l
}

// sendLog отправляет обычную строку
func sendLog(message string) {
	log.Println(message)
	if appLogger != nil {
		appLogger.OnLogReceived(message)
	}
}

// sendPrintf — это замена для log.Printf. Она сама соберет строку из формата и аргументов.
func sendPrintf(format string, v ...interface{}) {
	// Собираем красивую строку с помощью Спринтф
	message := fmt.Sprintf(format, v...)

	// Печатаем в консоль Go
	log.Println(message)

	// Передаем наверх в Kotlin
	if appLogger != nil {
		appLogger.OnLogReceived(message)
	}
}
