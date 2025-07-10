package core

import (
	"fmt"
	router "go.lumeweb.com/portal-router"
	"net/http"
	"sync"
)

const (
	ErrUnknownErrorKey ErrorType = "ErrUnknownError"
)

var (
	errorRegistry     = NewErrorRegistry()
	errorRegistryMu   sync.RWMutex
)

var _ error = (*Error)(nil)
var _ router.ResponseError = (*Error)(nil)

// ErrorType is a string type for error keys.
type ErrorType string

// ErrorRegistry manages error types and their associated data.
type ErrorRegistry struct {
	mu                sync.RWMutex
	namespaces        map[string]ErrorNamespace // Namespace -> ErrorNamespace
}

// ErrorNamespace holds the error definitions and HTTP status codes for a namespace.
type ErrorNamespace struct {
	errorDefinitions map[ErrorType]ErrorDefinition
	errorCodes       map[ErrorType]int
}

// ErrorDefinition holds the data associated with an error type.  It no longer contains the HttpStatus.
type ErrorDefinition struct {
	Key         ErrorType
	Message     string
	DefaultArgs []interface{} // Optional default arguments for the error message
}

// Error is the error object that will be returned.
type Error struct {
	Key       ErrorType     `json:"error"`     // A unique identifier for the error type
	Message   string        `json:"message"`   // Human-readable error message
	Err       error         `json:"-"`         // Underlying error, if any
	Args      []interface{} `json:"-"`         // Arguments used to format the error message
	Namespace string        `json:"namespace"` // The namespace of the error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// HttpStatus implements the router.ResponseError interface.
func (e *Error) HttpStatus() int {
	errorRegistryMu.RLock()
	defer errorRegistryMu.RUnlock()
	code, ok := errorRegistry.GetErrorCode(e.Namespace, e.Key)
	if !ok {
		return http.StatusInternalServerError
	}
	return code
}

// IsNamespace checks if the error belongs to the given namespace.
func (e *Error) IsNamespace(namespace string) bool {
	return e.Namespace == namespace
}

func (e *Error) IsErrorType(errType ErrorType) bool {
	return e.Key == errType
}

// NewErrorRegistry creates a new ErrorRegistry.
func NewErrorRegistry() *ErrorRegistry {
	return &ErrorRegistry{
		namespaces:        make(map[string]ErrorNamespace),
	}
}

// RegisterNamespace creates a new namespace in the registry.
func (r *ErrorRegistry) RegisterNamespace(namespace string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.namespaces[namespace]; exists {
		return fmt.Errorf("namespace '%s' already exists", namespace)
	}

	r.namespaces[namespace] = ErrorNamespace{
		errorDefinitions: make(map[ErrorType]ErrorDefinition),
		errorCodes:       make(map[ErrorType]int),
	}
	return nil
}

// RegisterDefaultErrorMessages registers a map of error definitions within a namespace.
func (r *ErrorRegistry) RegisterDefaultErrorMessages(namespace string, errorMap map[ErrorType]ErrorDefinition) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ns, exists := r.namespaces[namespace]
	if !exists {
		return fmt.Errorf("namespace '%s' does not exist", namespace)
	}

	for key, def := range errorMap {
		if _, exists := ns.errorDefinitions[key]; exists {
			return fmt.Errorf("error type '%s' already exists in namespace '%s'", key, namespace)
		}
		ns.errorDefinitions[key] = def
	}

	return nil
}

// RegisterErrorCodes registers a map of error codes (HTTP status codes) within a namespace.
func (r *ErrorRegistry) RegisterErrorCodes(namespace string, errorCodeMap map[ErrorType]int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ns, exists := r.namespaces[namespace]
	if !exists {
		return fmt.Errorf("namespace '%s' does not exist", namespace)
	}

	for key, code := range errorCodeMap {
		if _, exists := ns.errorCodes[key]; exists {
			return fmt.Errorf("error code for type '%s' already exists in namespace '%s'", key, namespace)
		}
		ns.errorCodes[key] = code
	}

	return nil
}

// GetErrorDefinition retrieves the ErrorDefinition for a given namespace and error type.
func (r *ErrorRegistry) GetErrorDefinition(namespace string, key ErrorType) (ErrorDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ns, exists := r.namespaces[namespace]
	if !exists {
		return ErrorDefinition{}, false
	}

	def, exists := ns.errorDefinitions[key]
	return def, exists
}

// GetErrorCode retrieves the HTTP status code for a given namespace and error type.
func (r *ErrorRegistry) GetErrorCode(namespace string, key ErrorType) (int, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ns, exists := r.namespaces[namespace]
	if !exists {
		return 0, false
	}

	code, exists := ns.errorCodes[key]
	if !exists {
		return http.StatusInternalServerError, false
	}
	return code, true
}

// NewError creates a new Error instance.  It formats the error message using the ErrorDefinition and provided arguments.
func (r *ErrorRegistry) NewError(namespace string, key ErrorType, err error, args ...interface{}) *Error {
	def, ok := r.GetErrorDefinition(namespace, key)
	if !ok {
		// Handle the case where the error type is not registered.  This is important!
		return &Error{
			Key:       ErrUnknownErrorKey,
			Message:   fmt.Sprintf("Unknown error type: %s (namespace: %s)", key, namespace),
			Err:       err,
			Namespace: namespace, // Set the namespace here
		}
	}

	// Use default arguments if none are provided
	if len(args) == 0 && len(def.DefaultArgs) > 0 {
		args = def.DefaultArgs
	}

	message := def.Message
	if len(args) > 0 {
		message = fmt.Sprintf(def.Message, args...) // Format the message
	}

	return &Error{
		Key:       key,
		Message:   message,
		Err:       err,
		Args:      args,
		Namespace: namespace,
	}
}

func RegisterNamespace(namespace string) error {
	errorRegistryMu.Lock()
	defer errorRegistryMu.Unlock()
	return errorRegistry.RegisterNamespace(namespace)
}

func RegisterErrorCodes(namespace string, errorCodeMap map[ErrorType]int) error {
	errorRegistryMu.Lock()
	defer errorRegistryMu.Unlock()
	return errorRegistry.RegisterErrorCodes(namespace, errorCodeMap)
}

func RegisterDefaultErrorMessages(namespace string, errorMap map[ErrorType]ErrorDefinition) error {
	errorRegistryMu.Lock()
	defer errorRegistryMu.Unlock()
	return errorRegistry.RegisterDefaultErrorMessages(namespace, errorMap)
}

// MustRegisterNamespace registers a namespace and panics if an error occurs.
func MustRegisterNamespace(namespace string) {
	if err := RegisterNamespace(namespace); err != nil {
		panic(fmt.Sprintf("Failed to register namespace '%s': %v", namespace, err))
	}
}

// MustRegisterDefaultErrorMessages registers default error messages and panics if an error occurs.
func MustRegisterDefaultErrorMessages(namespace string, errorMap map[ErrorType]ErrorDefinition) {
	if err := RegisterDefaultErrorMessages(namespace, errorMap); err != nil {
		panic(fmt.Sprintf("Failed to register default error messages for namespace '%s': %v", namespace, err))
	}
}

// MustRegisterErrorCodes registers error codes and panics if an error occurs.
func MustRegisterErrorCodes(namespace string, errorCodeMap map[ErrorType]int) {
	if err := RegisterErrorCodes(namespace, errorCodeMap); err != nil {
		panic(fmt.Sprintf("Failed to register error codes for namespace '%s': %v", namespace, err))
	}
}

// IsNamespaceError checks if the error is an error of the specified namespace.
func IsNamespaceError(err error, namespace string) bool {
	if err == nil {
		return false
	}

	namespacedError := AsNamespaceError(err, namespace)
	return namespacedError != nil
}

// AsNamespaceError casts the error to a *Error if possible and checks if it belongs to the specified namespace.
func AsNamespaceError(err error, namespace string) *Error {
	if err == nil {
		return nil
	}

	e, ok := err.(*Error)
	if !ok {
		return nil
	}

	if !e.IsNamespace(namespace) {
		return nil
	}

	return e
}

func ResetErrorRegistry() {
	errorRegistryMu.Lock()
	defer errorRegistryMu.Unlock()
	errorRegistry = NewErrorRegistry()
}
