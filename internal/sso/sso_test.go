package sso

import (
	"testing"
	"time"
)

func TestExpiry(t *testing.T) {
	got, err := Expiry("testdata", "corp")
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if _, err := Expiry("testdata", "missing"); err == nil {
		t.Fatal("want error for missing token")
	}
}
