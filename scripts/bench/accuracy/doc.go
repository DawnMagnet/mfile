//go:build !accuracy

// Package accuracy is the opt-in corpus gate. Files built with the accuracy
// tag are excluded here so that a plain `go test ./...` does not compile or
// run them. internal/detect does not exist on this branch; the check lives
// next to the other QA scripts until that package lands.
package accuracy
