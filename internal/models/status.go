package models

// StatusDisplayInfo contains display information for a lead status
type StatusDisplayInfo struct {
	DisplayName string
	BgColor     string
	TextColor   string
	BorderColor string
}

// GetStatusDisplayInfo returns display information for a given status
func GetStatusDisplayInfo(status string) StatusDisplayInfo {
	statusMap := map[string]StatusDisplayInfo{
		"lead_created": {
			DisplayName: "New Lead",
			BgColor:     "#E6E6E6",
			TextColor:   "#333",
			BorderColor: "#8C8C8C",
		},
		"test_booked": {
			DisplayName: "Test Booked",
			BgColor:     "#FFF4E6",
			TextColor:   "#8B6914",
			BorderColor: "#FFA500",
		},
		"tested": {
			DisplayName: "Tested",
			BgColor:     "#E6F3FF",
			TextColor:   "#0066CC",
			BorderColor: "#4EC6E0",
		},
		"offer_sent": {
			DisplayName: "Offer Sent",
			BgColor:     "#E6F7FF",
			TextColor:   "#0052A3",
			BorderColor: "#4EC6E0",
		},
		"booking_confirmed": {
			DisplayName: "Booking Confirmed",
			BgColor:     "#E6FFE6",
			TextColor:   "#006600",
			BorderColor: "#28a745",
		},
		"paid_full": {
			DisplayName: "Paid in Full",
			BgColor:     "#E6FFE6",
			TextColor:   "#006600",
			BorderColor: "#28a745",
		},
		"deposit_paid": {
			DisplayName: "Deposit Paid",
			BgColor:     "#FFF9E6",
			TextColor:   "#8B6914",
			BorderColor: "#FFA500",
		},
		"waiting_for_round": {
			DisplayName: "Waiting for Round",
			BgColor:     "#FFE6E6",
			TextColor:   "#CC0000",
			BorderColor: "#dc3545",
		},
		"schedule_assigned": {
			DisplayName: "Schedule Assigned",
			BgColor:     "#E6F3FF",
			TextColor:   "#0066CC",
			BorderColor: "#4EC6E0",
		},
		"ready_to_start": {
			DisplayName: "Ready to Start",
			BgColor:     "#E6FFE6",
			TextColor:   "#006600",
			BorderColor: "#28a745",
		},
		"cancelled": {
			DisplayName: "Cancelled",
			BgColor:     "#F5F5F5",
			TextColor:   "#666",
			BorderColor: "#8C8C8C",
		},
	}

	if info, ok := statusMap[status]; ok {
		return info
	}

	// Default for unknown status
	return StatusDisplayInfo{
		DisplayName: status,
		BgColor:     "#E6E6E6",
		TextColor:   "#333",
		BorderColor: "#8C8C8C",
	}
}
