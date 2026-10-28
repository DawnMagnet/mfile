//go:build amd64 && goexperiment.simd

package main

// maxProcs caps GOMAXPROCS unless the GOMAXPROCS environment variable is set.
// The Go backend keeps pooled buffers per P, so peak memory grows with
// GOMAXPROCS; with the AVX2/AVX-512 matmul kernels throughput stops
// improving at about 8.
const maxProcs = 8
