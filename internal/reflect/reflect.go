package reflect

import "reflect"

// CheckInterface checks if obj satisfies the target interface.
// It returns true if:
// 1. obj's type directly implements the interface.
// 2. A pointer to obj's type implements the interface.
// targetInterface must be the reflect.Type of the interface (e.g., reflect.TypeOf((*MyInterface)(nil)).Elem()).
func CheckInterface(obj any, targetInterface reflect.Type) bool {
	if obj == nil {
		return false // nil cannot satisfy any interface
	}

	v := reflect.ValueOf(obj)
	t := v.Type() // Get the concrete type stored in the 'any' (e.g., ScanConfig or *ScanConfig)

	// Check 1: Does the concrete type itself implement the interface?
	// This works if obj is T and T implements I, OR obj is *T and *T implements I.
	if t.Implements(targetInterface) {
		return true
	}

	// Check 2: If obj is not a pointer, does a pointer to its type implement the interface?
	// This specifically handles the case where obj is T, but only *T implements I.
	if v.Kind() != reflect.Pointer {
		ptrType := reflect.PointerTo(t)
		if ptrType.Implements(targetInterface) {
			return true
		}
	}

	// If neither the type nor a pointer to the type implements the interface.
	return false
}

// Helper to get interface types cleanly
func GetInterfaceType(prototype any) reflect.Type {
	t := reflect.TypeOf(prototype)
	// Ensure it's an interface, e.g., prototype was (*MyInterface)(nil)
	if t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Interface {
		return t.Elem()
	}
	// Handle case where prototype is zero value interface, e.g., (MyInterface)(nil)
	if t.Kind() == reflect.Interface {
		return t
	}
	// Or panic, depending on how strict you want to be
	panic("GetInterfaceType requires a nil pointer to an interface or a nil interface value")
}

// EnsureCompliantType takes an object and a target interface type.
// It returns an object that is suitable for satisfying the interface,
// potentially creating a pointer if needed (preferring original address if possible).
// It also returns a boolean indicating if the returned object actually satisfies the interface.
func EnsureCompliantType(obj any, targetInterface reflect.Type) (result any, ok bool) {
	if obj == nil {
		return nil, false // Nil cannot satisfy
	}

	v := reflect.ValueOf(obj)
	t := v.Type()

	// 1. Check if the original type already satisfies the interface
	if t.Implements(targetInterface) {
		// Yes, the original object (value or pointer) is fine.
		return obj, true
	}

	// 2. If not, and if it's a value type, check if a pointer *would* satisfy it.
	if v.Kind() != reflect.Pointer {
		ptrType := reflect.PointerTo(t)
		if ptrType.Implements(targetInterface) {
			// Pointer type satisfies it. We need a pointer.
			if v.CanAddr() {
				// Original value is addressable, return pointer to original.
				return v.Addr().Interface(), true
			} else {
				// Original value not addressable, return pointer to a copy.
				ptrToCopy := reflect.New(t)
				ptrToCopy.Elem().Set(v)
				return ptrToCopy.Interface(), true
			}
		}
	}

	// 3. If we reach here, neither the original type nor (if applicable) a pointer
	//    to the original type satisfies the interface.
	return obj, false // Return original object, but indicate failure
}
