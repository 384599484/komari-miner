//go:build windows

package probe

// queryOtherGPUs is a no-op on Windows: NVIDIA is handled by nvidia-smi and
// AMD's Windows tooling does not expose a stable machine-readable interface.
func queryOtherGPUs() ([]GPU, error) {
	return nil, errNoGPU
}
