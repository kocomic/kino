package server

import (
	"net/http"
	"slices"
	"strings"
)

// Keep method discovery and the registered API surface in one place.
func (s *Server) registerAPI(pattern string, handler http.HandlerFunc) {
	s.apiPatterns = append(s.apiPatterns, pattern)
	s.mux.HandleFunc(pattern, handler)
}
func (s *Server) allowedAPIMethods(path string) []string {
	var result []string
	for _, pattern := range s.apiPatterns {
		method, template, _ := strings.Cut(pattern, " ")
		actual, expected := strings.Split(path, "/"), strings.Split(template, "/")
		if len(actual) != len(expected) {
			continue
		}
		match := true
		for i := range actual {
			if strings.HasPrefix(expected[i], "{") && actual[i] != "" {
				continue
			}
			if actual[i] != expected[i] {
				match = false
				break
			}
		}
		if match {
			result = append(result, method)
			if method == "GET" {
				result = append(result, "HEAD")
			}
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}
