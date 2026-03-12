package grpc

import "context"

// ExtractMachineIDForTest exports extractMachineID for use in _test packages.
var ExtractMachineIDForTest = extractMachineID

// MachineIDFromContextForTest exports MachineIDFromContext for use in _test packages.
func MachineIDFromContextForTest(ctx context.Context) (string, error) {
	return MachineIDFromContext(ctx)
}
