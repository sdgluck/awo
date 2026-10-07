package awsconf

import (
	"reflect"
	"testing"
)

func TestLoad(t *testing.T) {
	got, err := Load("testdata/config")
	if err != nil {
		t.Fatal(err)
	}
	want := []Profile{
		{Name: "legacy", AccountID: "acct-legacy", Role: "Admin", Region: "eu-west-1", SessionKey: "https://example.awsapps.com/start"},
		{Name: "modern", AccountID: "acct-modern", Role: "ReadOnly", Region: "us-east-1", SessionKey: "corp"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}
