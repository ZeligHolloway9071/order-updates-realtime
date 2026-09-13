package orderfeed

import (
	"fmt"
	"net/url"

	"github.com/infrai-examples/order-updates-realtime/infrai"
)

// Notifier owns the order channels and pushes lifecycle events to them.
type Notifier struct {
	API       *infrai.Client
	AccountID string
}

// OpenOrder creates the channel for an order and mints the browser token for
// one shopper. The token is what the checkout page connects with; the server
// key stays on this side and is never sent to the browser.
func (n *Notifier) OpenOrder(orderID, clientID string, ttlSeconds int) (string, error) {
	ch := Channel(orderID)
	if err := n.API.Do("POST", "/realtime/channel/create", map[string]any{
		"channel": ch,
		"type":    "public",
	}, nil); err != nil {
		return "", err
	}

	var token struct {
		Token string `json:"token"`
	}
	err := n.API.Do("POST", "/realtime/token/issue", map[string]any{
		"client_id":    clientID,
		"channels":     []string{ch},
		"capabilities": []string{"subscribe"},
		"ttl_seconds":  ttlSeconds,
	}, &token)
	if err != nil {
		return "", err
	}
	return token.Token, nil
}

// Publish sends one decided notice to the order channel.
func (n *Notifier) Publish(notice Notice) error {
	if !notice.Send {
		return nil
	}
	return n.API.Do("POST", "/realtime/publish", map[string]any{
		"channel":    notice.Channel,
		"event":      notice.Event,
		"data":       notice.Data,
		"account_id": n.AccountID,
	}, nil)
}

// Watching reports how many of this order's viewers are connected right now,
// which is how the order service decides whether the same update also needs to
// go out by email.
func (n *Notifier) Watching(orderID string) (int, error) {
	var presence struct {
		Members []struct {
			ClientID string `json:"client_id"`
		} `json:"members"`
	}
	path := "/realtime/presence/get/" + url.PathEscape(Channel(orderID))
	if err := n.API.Do("GET", path, nil, &presence); err != nil {
		return 0, err
	}
	return len(presence.Members), nil
}

// Advance decides, publishes, and reports whether the shopper saw it live.
func (n *Notifier) Advance(u Update) (string, error) {
	notice, err := Decide(u)
	if err != nil {
		return "", err
	}
	if !notice.Send {
		return "skipped: " + notice.Reason, nil
	}
	if err := n.Publish(notice); err != nil {
		return "", err
	}
	watching, err := n.Watching(u.OrderID)
	if err != nil {
		return "", err
	}
	if watching == 0 {
		return fmt.Sprintf("%s published; no open tab, send it by email too", notice.Event), nil
	}
	return fmt.Sprintf("%s delivered live to %d viewer(s)", notice.Event, watching), nil
}
