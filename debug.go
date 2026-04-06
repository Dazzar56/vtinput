package vtinput

// Logger is a global function that can be set by the application to receive debug logs.
// This prevents vtinput from having a direct dependency on a specific logging implementation.
var Logger func(format string, a ...any)

// Log writes a debug message if the Logger is set.
func Log(format string, a ...any) {
	if Logger != nil {
		Logger(format, a...)
	}
}