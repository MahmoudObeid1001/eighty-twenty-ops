package models

import "testing"

func TestPurchasedTargetForPaymentCycle(t *testing.T) {
	tests := []struct {
		name          string
		isReturning   bool
		cycleBaseline int32
		bundleLevels  int32
		want          int32
	}{
		{
			name:         "first purchase uses bundle size",
			bundleLevels: 2,
			want:         2,
		},
		{
			name:          "single renewal extends the fixed cycle baseline",
			isReturning:   true,
			cycleBaseline: 1,
			bundleLevels:  1,
			want:          2,
		},
		{
			name:          "reprocessing after consumption does not manufacture a credit",
			isReturning:   true,
			cycleBaseline: 1,
			bundleLevels:  1,
			want:          2,
		},
		{
			name:          "returning bundle extends baseline by exact purchased bundle",
			isReturning:   true,
			cycleBaseline: 2,
			bundleLevels:  3,
			want:          5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := purchasedTargetForPaymentCycle(tt.isReturning, tt.cycleBaseline, tt.bundleLevels)
			if got != tt.want {
				t.Fatalf("purchased target = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPaymentCycleFullyFunded(t *testing.T) {
	tests := []struct {
		name       string
		totalPaid  int32
		finalPrice int32
		want       bool
	}{
		{name: "deposit does not grant entitlement", totalPaid: 600, finalPrice: 1250, want: false},
		{name: "full payment grants entitlement", totalPaid: 1250, finalPrice: 1250, want: true},
		{name: "invalid zero-value cycle does not grant entitlement", totalPaid: 1250, finalPrice: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paymentCycleFullyFunded(tt.totalPaid, tt.finalPrice)
			if got != tt.want {
				t.Fatalf("paymentCycleFullyFunded(%d, %d) = %v, want %v", tt.totalPaid, tt.finalPrice, got, tt.want)
			}
		})
	}
}
