package testing

import (
	"github.com/stretchr/testify/mock"
)

// WasMethodCalled checks if a method was called on the mock with the given arguments.
// Returns true if the method was called with matching arguments, false otherwise.
// Unlike testify's AssertCalled, this doesn't fail the test if not called.
func WasMethodCalled(m *mock.Mock, methodName string, arguments ...interface{}) bool {
	for _, call := range m.Calls {
		if call.Method == methodName {
			_, diffCount := call.Arguments.Diff(arguments)
			if diffCount == 0 {
				return true
			}
		}
	}
	return false
}

// WasMethodNotCalled checks if a method was NOT called on the mock with the given arguments.
// Returns true if the method was never called with matching arguments, false otherwise.
// Unlike testify's AssertNotCalled, this doesn't fail the test if called.
func WasMethodNotCalled(m *mock.Mock, methodName string, arguments ...interface{}) bool {
	return !WasMethodCalled(m, methodName, arguments...)
}
