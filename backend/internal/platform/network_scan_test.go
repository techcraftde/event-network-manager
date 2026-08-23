package platform

import (
	"strings"
	"testing"
)

func TestExpandScanRangesAcceptsPrivateEventSubnet(t *testing.T) {
	addresses, normalized, err := expandScanRanges([]string{"192.168.250.51-56"})
	if err != nil {
		t.Fatal(err)
	}
	if len(addresses) != 6 || addresses[0] != "192.168.250.51" || addresses[5] != "192.168.250.56" {
		t.Fatalf("addresses = %#v", addresses)
	}
	if len(normalized) != 1 || normalized[0] != "192.168.250.51-56" {
		t.Fatalf("normalized = %#v", normalized)
	}
}

func TestExpandScanRangesRejectsPublicOrOversizedRanges(t *testing.T) {
	if _, _, err := expandScanRanges([]string{"192.160.250.1-254"}); err == nil || !strings.Contains(err.Error(), "privates") {
		t.Fatalf("expected public range rejection, got %v", err)
	}
	if _, _, err := expandScanRanges([]string{"192.168.1.1-254,192.168.2.1-254,192.168.3.1-254,192.168.4.1-254,192.168.5.1-254"}); err == nil {
		t.Fatal("expected 1024-address limit")
	}
}
