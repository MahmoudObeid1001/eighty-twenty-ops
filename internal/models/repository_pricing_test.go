package models

import "testing"

func TestInferBundleLevelsFromPaidAmountUsesUpdatedSingleLevelThreshold(t *testing.T) {
	tests := []struct {
		name   string
		amount int32
		want   int32
	}{
		{name: "zero", amount: 0, want: 0},
		{name: "single upper bound", amount: 1250, want: 1},
		{name: "bundle two starts above single", amount: 1251, want: 2},
		{name: "bundle two upper bound", amount: 2400, want: 2},
		{name: "bundle three upper bound", amount: 3300, want: 3},
		{name: "bundle four above bundle three", amount: 3301, want: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferBundleLevelsFromPaidAmount(tt.amount); got != tt.want {
				t.Fatalf("inferBundleLevelsFromPaidAmount(%d) = %d, want %d", tt.amount, got, tt.want)
			}
		})
	}
}
