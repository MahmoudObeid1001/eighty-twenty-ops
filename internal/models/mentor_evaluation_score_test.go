package models

import "testing"

func TestComputeCollectiveClassScoreOptionalSkipsMissingManualMetrics(t *testing.T) {
	score := computeCollectiveClassScoreOptional(nil, nil, nil, intPtr(100), intPtr(100))
	if score != 100 {
		t.Fatalf("expected missing manual metrics to be neutral, got %d", score)
	}
}

func TestComputeCollectiveClassScoreOptionalNormalizesRecordedMetrics(t *testing.T) {
	score := computeCollectiveClassScoreOptional(intPtr(80), nil, nil, intPtr(100), intPtr(100))
	if score != 92 {
		t.Fatalf("expected score to be normalized over recorded metrics, got %d", score)
	}
}

func TestComputeCollectiveClassScoreOptionalReturnsZeroWithoutMetrics(t *testing.T) {
	score := computeCollectiveClassScoreOptional(nil, nil, nil, nil, nil)
	if score != 0 {
		t.Fatalf("expected empty score to be 0, got %d", score)
	}
}
