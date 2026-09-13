package orderfeed

import "testing"

func TestDecide(t *testing.T) {
	cases := []struct {
		name      string
		update    Update
		wantSend  bool
		wantEvent string
		wantData  map[string]any
		wantErr   bool
	}{
		{
			name:      "payment captured carries the receipt",
			update:    Update{OrderID: "A-1001", From: StageCheckout, To: StagePaid, Receipt: "https://shop.example/receipts/A-1001"},
			wantSend:  true,
			wantEvent: "order.paid",
			wantData: map[string]any{
				"order_id":    "A-1001",
				"stage":       "paid",
				"receipt_url": "https://shop.example/receipts/A-1001",
				"event_id":    "A-1001.paid",
			},
		},
		{
			name:      "shipment carries tracking",
			update:    Update{OrderID: "A-1001", From: StageFulfilling, To: StageShipped, Carrier: "ups", Tracking: "1Z999"},
			wantSend:  true,
			wantEvent: "order.shipped",
			wantData: map[string]any{
				"order_id": "A-1001",
				"stage":    "shipped",
				"carrier":  "ups",
				"tracking": "1Z999",
				"event_id": "A-1001.shipped",
			},
		},
		{
			name:     "replayed webhook does not walk the order backwards",
			update:   Update{OrderID: "A-1001", From: StageShipped, To: StagePaid, Receipt: "https://shop.example/receipts/A-1001"},
			wantSend: false,
		},
		{
			name:     "same stage twice publishes once",
			update:   Update{OrderID: "A-1001", From: StageShipped, To: StageShipped},
			wantSend: false,
		},
		{
			name:      "cancellation always goes out",
			update:    Update{OrderID: "A-1001", From: StageShipped, To: StageCancelled},
			wantSend:  true,
			wantEvent: "order.cancelled",
			wantData:  map[string]any{"order_id": "A-1001", "stage": "cancelled", "event_id": "A-1001.cancelled"},
		},
		{
			name:    "paid without a receipt is rejected",
			update:  Update{OrderID: "A-1001", From: StageCheckout, To: StagePaid},
			wantErr: true,
		},
		{
			name:    "unknown stage is rejected",
			update:  Update{OrderID: "A-1001", From: StageCheckout, To: Stage("refunded")},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Decide(tc.update)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got notice %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Send != tc.wantSend {
				t.Fatalf("Send = %v, want %v (reason %q)", got.Send, tc.wantSend, got.Reason)
			}
			if !tc.wantSend {
				return
			}
			if got.Event != tc.wantEvent {
				t.Errorf("Event = %q, want %q", got.Event, tc.wantEvent)
			}
			if got.Channel != "order.a-1001" {
				t.Errorf("Channel = %q, want %q", got.Channel, "order.a-1001")
			}
			if len(got.Data) != len(tc.wantData) {
				t.Fatalf("Data = %v, want %v", got.Data, tc.wantData)
			}
			for k, want := range tc.wantData {
				if got.Data[k] != want {
					t.Errorf("Data[%q] = %v, want %v", k, got.Data[k], want)
				}
			}
		})
	}
}
