package order

import "testing"

func TestStatusConstants(t *testing.T) {
	if StatusNew == "" || StatusFilled == "" {
		t.Fatalf("status constants not set")
	}
}
