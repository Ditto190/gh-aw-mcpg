package difc

// ShouldBypassCoarseDeny returns true when a coarse-grained deny should still
// proceed to backend execution so Phase 5 can enforce per-item policy.
func ShouldBypassCoarseDeny(operation OperationType) bool {
	return operation == OperationRead
}

// ShouldCallLabelResponse returns true when guards should label response data
// for possible fine-grained filtering.
func ShouldCallLabelResponse(operation OperationType, enforcementMode EnforcementMode) bool {
	isPureWrite := operation == OperationWrite
	return !isPureWrite && (operation != OperationReadWrite || enforcementMode != EnforcementStrict)
}

// ShouldBlockFilteredResponse returns true when filtered items should block the
// whole response instead of returning a partially filtered result.
func ShouldBlockFilteredResponse(enforcementMode EnforcementMode, filteredCount int) bool {
	return enforcementMode == EnforcementStrict && filteredCount > 0
}

// ShouldAccumulateReadLabels returns true when read labels should be
// accumulated back into the agent label set.
func ShouldAccumulateReadLabels(operation OperationType, enforcementMode EnforcementMode) bool {
	return operation != OperationWrite && enforcementMode == EnforcementPropagate
}
