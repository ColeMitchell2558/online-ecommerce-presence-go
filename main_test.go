package main

import "testing"

func TestOrderEventTransitions(t *testing.T) {
	tests := []struct{ current, want string }{{"checkout", "fulfillment"}, {"fulfillment", "receipt"}, {"receipt", "customer_update"}}
	for _, tt := range tests {
		got, ok := nextOrderEvent(tt.current)
		if !ok || got != tt.want {
			t.Fatalf("%s -> %q, %v; want %q, true", tt.current, got, ok, tt.want)
		}
	}
	if _, ok := nextOrderEvent("customer_update"); ok {
		t.Fatal("customer_update must be terminal")
	}
}
