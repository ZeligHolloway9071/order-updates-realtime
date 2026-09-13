package orderfeed

import (
	"fmt"
	"strings"
)

// Stage is a point in the order lifecycle a shopper can observe.
type Stage string

const (
	StageCheckout   Stage = "checkout"
	StagePaid       Stage = "paid"
	StageFulfilling Stage = "fulfilling"
	StageShipped    Stage = "shipped"
	StageDelivered  Stage = "delivered"
	StageCancelled  Stage = "cancelled"
)

var rank = map[Stage]int{
	StageCheckout:   1,
	StagePaid:       2,
	StageFulfilling: 3,
	StageShipped:    4,
	StageDelivered:  5,
}

// Update is what the order service hands to the notifier.
type Update struct {
	OrderID  string
	Customer string
	From     Stage
	To       Stage
	Receipt  string // receipt URL, set once payment is captured
	Carrier  string
	Tracking string
}

// Notice is the realtime message a shopper should receive, or a decision not
// to send one.
type Notice struct {
	Send    bool
	Reason  string
	Event   string
	Channel string
	Data    map[string]any
}

// Channel names the per-order stream. Both the server publishing to it and the
// browser token scoped to it derive the name here, so the two sides cannot
// drift apart.
func Channel(orderID string) string {
	return "order." + strings.ToLower(orderID)
}

// Decide turns a lifecycle transition into the notice to publish.
//
// A cancellation always reaches the shopper. A move backwards (a retried
// webhook replaying an older stage) is dropped so the order view never walks
// back from "shipped" to "paid". Everything else becomes one event carrying
// the fields that stage actually has.
func Decide(u Update) (Notice, error) {
	if u.OrderID == "" {
		return Notice{}, fmt.Errorf("orderfeed: update has no order id")
	}
	n := Notice{Channel: Channel(u.OrderID)}

	if u.To == StageCancelled {
		n.Send = true
		n.Event = "order.cancelled"
		n.Data = map[string]any{
			"order_id": u.OrderID,
			"stage":    string(StageCancelled),
			"event_id": u.OrderID + ".cancelled",
		}
		return n, nil
	}

	to, ok := rank[u.To]
	if !ok {
		return Notice{}, fmt.Errorf("orderfeed: unknown stage %q", u.To)
	}
	if from, seen := rank[u.From]; seen && to <= from {
		n.Reason = fmt.Sprintf("replay of %s behind %s", u.To, u.From)
		return n, nil
	}

	n.Send = true
	n.Event = "order." + string(u.To)
	n.Data = map[string]any{"order_id": u.OrderID, "stage": string(u.To)}
	switch u.To {
	case StagePaid:
		if u.Receipt == "" {
			return Notice{}, fmt.Errorf("orderfeed: order %s reached paid without a receipt", u.OrderID)
		}
		n.Data["receipt_url"] = u.Receipt
	case StageShipped:
		n.Data["carrier"] = u.Carrier
		n.Data["tracking"] = u.Tracking
	}
	// The event id makes a retried publish land once: replaying the same
	// transition carries the same id the client already rendered.
	n.Data["event_id"] = fmt.Sprintf("%s.%s", u.OrderID, u.To)
	return n, nil
}
