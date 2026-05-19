// Package platform abstracts service-manager integration so that the rest of
// the application compiles and runs on any OS. On Windows it wraps the
// Service Control Manager via [internal/winsvc]; on other platforms the
// service functions are no-ops that always report "not a service".
package platform

// Starter is implemented by the application and accepted by [RunAsService].
// It is satisfied by *App in main.go.
type Starter interface {
	Start() error
	Stop()
}
