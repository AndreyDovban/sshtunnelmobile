// tunnel/listener.go
package tunnel

// StatusListener — gomobile превратит его в Java/Kotlin интерфейс.
// Мы будем вызывать его методы, когда состояние туннеля меняется изнутри Go.
type StatusListener interface {
	// Вызывается при непредвиденном обрыве связи (например, упал пинг)
	OnConnectionLost(reason string)
}

// Глобальная переменная для хранения слушателя событий
var appStatusListener StatusListener

// SetStatusListener позволяет Kotlin передать свой слушатель внутрь Go-ядра
func SetStatusListener(l StatusListener) {
	appStatusListener = l
}
