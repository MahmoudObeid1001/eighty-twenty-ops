package handlers

import (
	"testing"

	"eighty-twenty-ops/internal/models"
)

func TestShouldRequireFinalPriceForSave(t *testing.T) {
	tests := []struct {
		name                string
		newStage            string
		currentStatus       string
		offerPricingChanged bool
		want                bool
	}{
		{
			name:                "notes-only save on offer-sent lead skips validation",
			newStage:            models.StageOfferSent,
			currentStatus:       "offer_sent",
			offerPricingChanged: false,
			want:                false,
		},
		{
			name:                "notes-only save on deposit-paid lead skips validation",
			newStage:            models.StageBookingConfirmedDeposit,
			currentStatus:       "deposit_paid",
			offerPricingChanged: false,
			want:                false,
		},
		{
			name:                "explicit offer pricing edit still validates",
			newStage:            models.StageOfferSent,
			currentStatus:       "offer_sent",
			offerPricingChanged: true,
			want:                true,
		},
		{
			name:                "moving from tested to offer stage validates",
			newStage:            models.StageOfferSent,
			currentStatus:       "tested",
			offerPricingChanged: false,
			want:                true,
		},
		{
			name:                "pre-offer payment stage still validates",
			newStage:            models.StageBookingConfirmedPaidFull,
			currentStatus:       "tested",
			offerPricingChanged: false,
			want:                true,
		},
		{
			name:                "pre-offer tested stage does not validate",
			newStage:            models.StageTested,
			currentStatus:       "tested",
			offerPricingChanged: false,
			want:                false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRequireFinalPriceForSave(tt.newStage, tt.currentStatus, tt.offerPricingChanged)
			if got != tt.want {
				t.Fatalf("shouldRequireFinalPriceForSave(%q, %q, %v) = %v, want %v",
					tt.newStage, tt.currentStatus, tt.offerPricingChanged, got, tt.want)
			}
		})
	}
}
