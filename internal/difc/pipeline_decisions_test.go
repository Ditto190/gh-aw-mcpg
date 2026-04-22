package difc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldBypassCoarseDeny(t *testing.T) {
	assert.True(t, ShouldBypassCoarseDeny(OperationRead))
	assert.False(t, ShouldBypassCoarseDeny(OperationWrite))
	assert.False(t, ShouldBypassCoarseDeny(OperationReadWrite))
}

func TestShouldCallLabelResponse(t *testing.T) {
	assert.False(t, ShouldCallLabelResponse(OperationWrite, EnforcementStrict))
	assert.False(t, ShouldCallLabelResponse(OperationReadWrite, EnforcementStrict))
	assert.True(t, ShouldCallLabelResponse(OperationRead, EnforcementStrict))
	assert.True(t, ShouldCallLabelResponse(OperationReadWrite, EnforcementFilter))
	assert.True(t, ShouldCallLabelResponse(OperationReadWrite, EnforcementPropagate))
}

func TestShouldBlockFilteredResponse(t *testing.T) {
	assert.True(t, ShouldBlockFilteredResponse(EnforcementStrict, 1))
	assert.False(t, ShouldBlockFilteredResponse(EnforcementStrict, 0))
	assert.False(t, ShouldBlockFilteredResponse(EnforcementFilter, 3))
	assert.False(t, ShouldBlockFilteredResponse(EnforcementPropagate, 2))
}

func TestShouldAccumulateReadLabels(t *testing.T) {
	assert.True(t, ShouldAccumulateReadLabels(OperationRead, EnforcementPropagate))
	assert.True(t, ShouldAccumulateReadLabels(OperationReadWrite, EnforcementPropagate))
	assert.False(t, ShouldAccumulateReadLabels(OperationWrite, EnforcementPropagate))
	assert.False(t, ShouldAccumulateReadLabels(OperationRead, EnforcementStrict))
	assert.False(t, ShouldAccumulateReadLabels(OperationRead, EnforcementFilter))
}
