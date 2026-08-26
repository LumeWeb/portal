package core

import (
	"encoding/json"
	"fmt"
	router "go.lumeweb.com/portal-router"
	"net/http"
	"strings"
	"sync"

	"github.com/samber/lo"
)

const (
	ErrUnknownErrorKey ErrorType = "ErrUnknownError"
)

var (
	errorRegistry   = NewErrorRegistry()
	errorRegistryMu sync.RWMutex
)

var _ error = (*Error)(nil)
var _ router.ResponseError = (*Error)(nil)
var _ json.Marshaler = (*Error)(nil)

// ErrorType is a string type for error keys.
type ErrorType string

// ErrorRegistry manages error types and their associated data.
type ErrorRegistry struct {
	mu         sync.RWMutex
	namespaces map[string]ErrorNamespace // Namespace -> ErrorNamespace
}


// ErrorDefinition holds the data associated with an error type.  It no longer contains the HttpStatus.
type ErrorDefinition struct {
	Key         ErrorType
	Message     string
	DefaultArgs []any // Optional default arguments for the error message
}

// Error is the error object that will be returned.
type Error struct {
	Key       ErrorType // A unique identifier for the error type
	Message   string    // Human-readable error message
	Err       error     // Underlying error, if any
	Args      []any     // Arguments used to format the error message
	Namespace string    // The namespace of the error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// MarshalJSON serializes Error into the API error response format:
// {"error":{"reason":"...","details":"..."}}.
func (e *Error) MarshalJSON() ([]byte, error) {
	if e == nil {
		return json.Marshal(errorResponseBody{Error: errorDetail{Reason: "Unknown"}})
	}

	reason := string(e.Key)
	reason = strings.TrimPrefix(reason, "ErrKey")
	reason = strings.TrimPrefix(reason, "Err")
	// Convert SCREAMING_SNAKE_CASE to PascalCase
	if strings.Contains(reason, "_") {
		parts := strings.Split(reason, "_")
		for i, p := range parts {
			if p != "" {
				parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
			}
		}
		reason = strings.Join(parts, "")
	}

	return json.Marshal(errorResponseBody{
		Error: errorDetail{
			Reason:  reason,
			Details: e.Message,
		},
	})
}

// errorDetail is the canonical structured error detail format.
type errorDetail struct {
	Reason  string `json:"reason"`
	Details string `json:"details,omitempty"`
}

// errorResponseBody wraps errorDetail under the "error" key.
type errorResponseBody struct {
	Error errorDetail `json:"error"`
}

// Unwrap enables errors.Is/As to traverse the underlying cause.
func (e *Error) Unwrap() error {
	return e.Err
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

// ErrorNamespace contains all exported error namespace data
type ErrorNamespace struct {
	Definitions map[ErrorType]ErrorDefinition
	Codes       map[ErrorType]int
}

// ErrorNamespaces is a map of namespace names to their ErrorNamespace data
type ErrorNamespaces map[string]ErrorNamespace

// NewErrorRegistry creates a new ErrorRegistry.
func NewErrorRegistry() *ErrorRegistry {
	return &ErrorRegistry{
		namespaces: make(map[string]ErrorNamespace), 
	}
}

// ExportAllNamespaces exports all registered error namespaces with their error definitions and codes
func (r *ErrorRegistry) ExportAllNamespaces() ErrorNamespaces {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return lo.MapValues(r.namespaces, func(ns ErrorNamespace, _ string) ErrorNamespace {
		// Deep copy Definitions map
		defsCopy := lo.MapValues(ns.Definitions, func(def ErrorDefinition, _ ErrorType) ErrorDefinition {
			return ErrorDefinition{
				Key:         def.Key,
				Message:     def.Message,
				DefaultArgs: append([]any(nil), def.DefaultArgs...), // Deep copy slice
			}
		})

		// Deep copy Codes map
		codesCopy := lo.MapValues(ns.Codes, func(code int, _ ErrorType) int {
			return code
		})

		return ErrorNamespace{
			Definitions: defsCopy,
			Codes:       codesCopy,
		}
	})
}

// ImportNamespaces imports error namespaces with their error definitions and codes.
// Any existing definitions/codes for the same keys will be silently overwritten.
// Returns nil on success or an error if the import fails.
func (r *ErrorRegistry) ImportNamespaces(importData ErrorNamespaces) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for nsName, data := range importData {
		// Get existing namespace or create new one
		ns, exists := r.namespaces[nsName]
		if !exists {
			ns = ErrorNamespace{
				Definitions: make(map[ErrorType]ErrorDefinition),
				Codes:       make(map[ErrorType]int),
			}
		} else {
			// Ensure existing maps are initialized
			if ns.Definitions == nil {
				ns.Definitions = make(map[ErrorType]ErrorDefinition)
			}
			if ns.Codes == nil {
				ns.Codes = make(map[ErrorType]int)
			}
		}

		// Copy definitions
		for key, def := range data.Definitions {
			ns.Definitions[key] = def
		}
		
		// Copy codes
		for key, code := range data.Codes {
			ns.Codes[key] = code
		}

		// Store the updated namespace
		r.namespaces[nsName] = ns
	}
	return nil
}

// RegisterNamespace creates a new namespace in the registry.
func (r *ErrorRegistry) RegisterNamespace(namespace string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.namespaces[namespace]; exists {
		return fmt.Errorf("namespace '%s' already exists", namespace)
	}

	r.namespaces[namespace] = ErrorNamespace{
		Definitions: make(map[ErrorType]ErrorDefinition),
		Codes:       make(map[ErrorType]int),
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
		if def.Key == "" {
			def.Key = key
		} else if def.Key != key {
			return fmt.Errorf("definition key mismatch: map key=%q def.Key=%q in namespace %q", key, def.Key, namespace)
		}
		if _, exists := ns.Definitions[key]; exists {
			return fmt.Errorf("error type '%s' already exists in namespace '%s'", key, namespace)
		}
		ns.Definitions[key] = def
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
		if code < 100 || code > 599 {
			return fmt.Errorf("invalid HTTP status code %d for type '%s' in namespace '%s'", code, key, namespace)
		}
		if _, exists := ns.Codes[key]; exists {
			return fmt.Errorf("error code for type '%s' already exists in namespace '%s'", key, namespace)
		}
		ns.Codes[key] = code
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

	def, exists := ns.Definitions[key]
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

	code, exists := ns.Codes[key]
	if !exists {
		return http.StatusInternalServerError, false
	}
	return code, true
}

// NewError creates a new Error instance.  It formats the error message using the ErrorDefinition and provided arguments.
func (r *ErrorRegistry) NewError(namespace string, key ErrorType, err error, args ...any) *Error {
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
	} else if err != nil && countFormattedVerbs(def.Message) == 1 {
		// Convention: callers pass the descriptive detail via err. Bridge it into
		// the single format verb so the template reads e.g.
		// "Invalid request parameter: a domain is required...".
		// Only bridge when the template resolves to exactly one verb so literal
		// percent signs and multi-verb templates are left untouched.
		message = fmt.Sprintf(def.Message, err.Error())
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

// ReplaceAllErrorNamespaces replaces all error namespaces with the provided ones
func ReplaceAllErrorNamespaces(namespaces ErrorNamespaces) error {
	errorRegistryMu.Lock()
	defer errorRegistryMu.Unlock()
	
	// Create new registry and import namespaces
	newRegistry := NewErrorRegistry()
	if err := newRegistry.ImportNamespaces(namespaces); err != nil {
		return err
	}
	
	// Atomically replace the registry
	errorRegistry = newRegistry
	return nil
}

// ExportAllErrorNamespaces exports all error namespaces from the global error registry
func ExportAllErrorNamespaces() ErrorNamespaces {
	errorRegistryMu.RLock()
	defer errorRegistryMu.RUnlock()
	return errorRegistry.ExportAllNamespaces()
}

// ImportErrorNamespaces imports error namespaces into the global error registry
func ImportErrorNamespaces(importData ErrorNamespaces) error {
	errorRegistryMu.Lock()
	defer errorRegistryMu.Unlock()
	return errorRegistry.ImportNamespaces(importData)
}

// NewError creates a new Error instance using the global error registry.
// It formats the error message using the ErrorDefinition and provided arguments.
func NewError(namespace string, key ErrorType, err error, args ...any) *Error {
	errorRegistryMu.RLock()
	defer errorRegistryMu.RUnlock()
	return errorRegistry.NewError(namespace, key, err, args...)
}

// countFormattedVerbs returns the number of format verbs (directives that
// consume an argument) in s. A literal percent sign ("%%") is not a verb, and
// malformed trailing "%" without a verb is ignored.
func countFormattedVerbs(s string) int {
	count := 0
	for i := 0; i < len(s); {
		if s[i] != '%' {
			i++
			continue
		}
		i++ // consume '%'
		if i < len(s) && s[i] == '%' {
			i++ // literal percent, not a verb
			continue
		}
		// Skip flags.
		for i < len(s) && strings.ContainsRune("#0+- ", rune(s[i])) {
			i++
		}
		// Skip width (digits or '*').
		for i < len(s) && (s[i] == '*' || (s[i] >= '0' && s[i] <= '9')) {
			i++
		}
		// Skip precision ('.' followed by digits or '*').
		if i < len(s) && s[i] == '.' {
			i++
			for i < len(s) && (s[i] == '*' || (s[i] >= '0' && s[i] <= '9')) {
				i++
			}
		}
		// Skip argument index '[n]'.
		if i < len(s) && s[i] == '[' {
			for i < len(s) && s[i] != ']' {
				i++
			}
			if i < len(s) {
				i++ // consume ']'
			}
		}
		if i < len(s) {
			count++
			i++
		}
	}
	return count
}
