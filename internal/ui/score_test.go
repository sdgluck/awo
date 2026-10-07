package ui

import "testing"

func TestScore(t *testing.T) {
	sub, _ := Score("stg", "fink-stg")
	seq, ok := Score("fs", "fink-stg")
	if !ok || seq >= sub {
		t.Fatalf("substring %d should beat subsequence %d", sub, seq)
	}
	early, _ := Score("fink", "fink-stg")
	if early <= sub {
		t.Fatal("earlier substring should rank higher")
	}
	if _, ok := Score("zz", "fink-stg"); ok {
		t.Fatal("non-match should be excluded")
	}
}
