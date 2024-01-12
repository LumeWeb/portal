package router

import "net/http"

type ProtocolRouter map[string]http.Handler

// Implement the ServeHTTP method on our new type
func (hs ProtocolRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if handler := hs[r.Host]; handler != nil {
		handler.ServeHTTP(w, r)
	} else {
		http.Error(w, "Forbidden", 403) // Or Redirect?
	}
}
