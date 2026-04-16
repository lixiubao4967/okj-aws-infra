// Package ptr provides generic pointer helpers.
package ptr

// To returns a pointer to the given value.
// Use this to pass literal values where a pointer is required,
// e.g. ptr.To(2) instead of: n := 2; return &n
func To[T any](v T) *T {
	return &v
}
