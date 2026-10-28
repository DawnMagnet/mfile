//go:build !(amd64 && goexperiment.simd)

package cli

// maxProcs is 0 (no cap): without the SIMD matmul kernels inference is
// compute bound and keeps scaling with every core.
const maxProcs = 0
