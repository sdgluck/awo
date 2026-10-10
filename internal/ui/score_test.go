package ui

import (
	"testing"

	"github.com/sdgluck/awo/internal/awsconf"
)

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

func TestMatch(t *testing.T) {
	p := awsconf.Profile{Name: "fink-stg", AccountID: "acct-4567", Role: "Admin", Region: "eu-west-1"}
	name, _ := Score("fs", "fink-stg")
	acct, ok := Match("4567", p)
	if !ok || acct >= name {
		t.Fatalf("name subsequence %d should beat account match %d", name, acct)
	}
	if _, ok := Match("eu-west", p); !ok {
		t.Fatal("region should match")
	}
	if _, ok := Match("zz", p); ok {
		t.Fatal("non-match should be excluded")
	}
}

func TestIsProd(t *testing.T) {
	for name, want := range map[string]bool{
		"fink-prd": true, "prod2-eu": true, "Production": true,
		"product-dev": false, "fink-stg": false, "reprod": false,
	} {
		if IsProd(name) != want {
			t.Errorf("IsProd(%q) = %v", name, !want)
		}
	}
}
