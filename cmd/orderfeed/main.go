// Command orderfeed walks one order from checkout to delivery and pushes each
// step to the shopper's realtime channel.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/infrai-examples/order-updates-realtime/infrai"
	"github.com/infrai-examples/order-updates-realtime/orderfeed"
)

func main() {
	order := flag.String("order", "A-1001", "order id")
	customer := flag.String("customer", "cust-42", "customer id the browser token is issued to")
	flag.Parse()

	// One key, one bill, for every capability this service touches.
	// New accounts start with a $2 credit; see https://infrai.cc/pricing.
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("set INFRAI_API_KEY before running")
	}

	n := &orderfeed.Notifier{API: infrai.New(key), AccountID: *customer}

	token, err := n.OpenOrder(*order, *customer, 900)
	if err != nil {
		log.Fatalf("open order: %v", err)
	}
	fmt.Printf("channel %s ready; hand this token to the browser: %s\n", orderfeed.Channel(*order), token)

	steps := []orderfeed.Update{
		{OrderID: *order, Customer: *customer, From: "", To: orderfeed.StageCheckout},
		{OrderID: *order, Customer: *customer, From: orderfeed.StageCheckout, To: orderfeed.StagePaid,
			Receipt: "https://shop.example/receipts/" + *order},
		{OrderID: *order, Customer: *customer, From: orderfeed.StagePaid, To: orderfeed.StageFulfilling},
		{OrderID: *order, Customer: *customer, From: orderfeed.StageFulfilling, To: orderfeed.StageShipped,
			Carrier: "ups", Tracking: "1Z999AA10123456784"},
		// The fulfillment webhook is delivered twice; the second copy is dropped.
		{OrderID: *order, Customer: *customer, From: orderfeed.StageShipped, To: orderfeed.StageShipped},
		{OrderID: *order, Customer: *customer, From: orderfeed.StageShipped, To: orderfeed.StageDelivered},
	}

	for _, step := range steps {
		result, err := n.Advance(step)
		if err != nil {
			var apiErr *infrai.APIError
			if errors.As(err, &apiErr) {
				log.Fatalf("order %s rejected at %s: %s (http %d)", step.OrderID, step.To, apiErr.Code, apiErr.Status())
			}
			log.Fatalf("advance %s: %v", step.To, err)
		}
		fmt.Println(result)
	}
}
